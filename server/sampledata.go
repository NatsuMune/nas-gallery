package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"math/rand"
	"os"
	"path/filepath"
)

// cmdSampleData generates a synthetic media tree (small PNG photos plus
// handcrafted minimal MP4/WebM videos) for manual testing of the server.
func cmdSampleData(args []string) error {
	fs := flag.NewFlagSet("sampledata", flag.ContinueOnError)
	out := fs.String("out", "sample-media", "output directory (created)")
	photos := fs.Int("photos", 24, "number of PNG photos")
	videos := fs.Int("videos", 3, "number of videos (alternating mp4/webm)")
	seed := fs.Int64("seed", 1, "deterministic RNG seed")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *photos < 0 || *videos < 0 {
		return fmt.Errorf("photos and videos must be >= 0")
	}
	return generateSampleData(*out, *photos, *videos, *seed)
}

func generateSampleData(outDir string, nPhotos, nVideos int, seed int64) error {
	rng := rand.New(rand.NewSource(seed))
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}

	photoFolders := []string{
		"Photos/2023/summer",
		"Photos/2023/winter",
		"Photos/2024/spring",
		"Photos/2024/autumn",
		"Photos/2025/holiday",
	}
	created := 0
	for i := 0; i < nPhotos; i++ {
		dir := filepath.Join(outDir, photoFolders[i%len(photoFolders)])
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		name := fmt.Sprintf("IMG_%04d.png", i+1)
		w := 160 + rng.Intn(480)
		h := 120 + rng.Intn(360)
		if err := writeSamplePNG(filepath.Join(dir, name), w, h, rng); err != nil {
			return err
		}
		created++
	}

	videoFolders := []string{"Videos/PS5", "Videos/WebM"}
	for i := 0; i < nVideos; i++ {
		dir := filepath.Join(outDir, videoFolders[i%len(videoFolders)])
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		durMs := 15000 + 100*i
		if i%2 == 0 {
			name := fmt.Sprintf("clip_%02d.mp4", i/2)
			data := buildMinimalMP4(uint32(durMs), 1920, 1080, 128*1024)
			if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
				return err
			}
		} else {
			name := fmt.Sprintf("demo_%02d.webm", i/2)
			data := buildMinimalWebM(float64(durMs))
			if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
				return err
			}
		}
		created++
	}

	// A few entries the scanner must exclude (for manual verification).
	excluded := map[string]string{
		filepath.Join(outDir, "Photos/2023/summer/.DS_Store"):              "finder junk",
		filepath.Join(outDir, "Photos/2023/@eaDir/SYNOPHOTO:DB_Thumbnail"): "synology thumbs",
		filepath.Join(outDir, "Misc/Thumbs.db"):                            "windows thumbs",
		filepath.Join(outDir, "Videos/.hidden.mkv"):                        "dotfile",
	}
	for path, why := range excluded {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(why), 0o644); err != nil {
			return err
		}
	}

	fmt.Printf("created %d sample media files under %s\n", created, outDir)
	fmt.Printf("point mediaPaths at %q in config.yaml, then run \"nas-gallery serve\" and trigger a scan\n", outDir)
	return nil
}

// writeSamplePNG renders a small deterministic gradient photo.
func writeSamplePNG(path string, w, h int, rng *rand.Rand) error {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	baseHue := rng.Float64() * 2 * math.Pi
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			hue := baseHue + float64(x)/float64(w)*1.2 + float64(y)/float64(h)*0.6
			img.SetRGBA(x, y, hsv(hue, 0.55+0.25*math.Sin(float64(y)/12), 0.85+0.15*math.Cos(float64(x)/17)))
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

// hsv converts hue [0,2π), saturation and value in [0,1] to an RGBA color.
func hsv(h, s, v float64) color.RGBA {
	i := math.Floor(h / (math.Pi / 3))
	f := h/(math.Pi/3) - i
	p := v * (1 - s)
	q := v * (1 - s*f)
	t := v * (1 - s*(1-f))
	var r, g, b float64
	switch int(i) % 6 {
	case 0:
		r, g, b = v, t, p
	case 1:
		r, g, b = q, v, p
	case 2:
		r, g, b = p, v, t
	case 3:
		r, g, b = p, q, v
	case 4:
		r, g, b = t, p, v
	default:
		r, g, b = v, p, q
	}
	return color.RGBA{R: uint8(r * 255), G: uint8(g * 255), B: uint8(b * 255), A: 255}
}

// --- handcrafted minimal container fixtures -----------------------------------
//
// These builders create tiny but structurally valid MP4 and WebM files with a
// proper moov/mvhd/tkhd or Segment/Info/Duration so the native metadata
// parsers (and the scanner) have real data to work with. No actual frames are
// needed; mdat carries deterministic filler bytes so streaming/range tests
// have a realistic body. They are shared with the unit tests.

// buildMinimalMP4 constructs: ftyp + moov{mvhd, trak{tkhd}} + mdat(filler).
func buildMinimalMP4(durationMs uint32, width, height uint16, filler int) []byte {
	timescale := uint32(1000) // ms

	mvhd := make([]byte, 0, 100)
	mvhd = append(mvhd, 0, 0, 0, 0)          // version 0 + flags
	mvhd = append(mvhd, u32(0)...)           // creation time
	mvhd = append(mvhd, u32(0)...)           // modification time
	mvhd = append(mvhd, u32(timescale)...)   // timescale
	mvhd = append(mvhd, u32(durationMs)...)  // duration (in timescale units)
	mvhd = append(mvhd, u32(0x00010000)...)  // rate 1.0 (16.16)
	mvhd = append(mvhd, 0x01, 0x00)          // volume 1.0 (8.8)
	mvhd = append(mvhd, 0, 0)                // reserved
	mvhd = append(mvhd, make([]byte, 8)...)  // reserved
	mvhd = append(mvhd, identityMatrix()...) // matrix
	mvhd = append(mvhd, make([]byte, 24)...) // predefined
	mvhd = append(mvhd, u32(2)...)           // next_track_id

	tkhd := make([]byte, 0, 84)
	tkhd = append(tkhd, 0, 0, 0, 7)                 // version 0, flags 0x7 (enabled|in_movie|duration)
	tkhd = append(tkhd, u32(0)...)                  // creation
	tkhd = append(tkhd, u32(0)...)                  // modification
	tkhd = append(tkhd, u32(1)...)                  // track_id
	tkhd = append(tkhd, u32(0)...)                  // reserved
	tkhd = append(tkhd, u32(durationMs)...)         // duration
	tkhd = append(tkhd, make([]byte, 8)...)         // reserved
	tkhd = append(tkhd, 0, 0)                       // layer
	tkhd = append(tkhd, 0, 0)                       // alternate group
	tkhd = append(tkhd, 0, 0)                       // volume
	tkhd = append(tkhd, 0, 0)                       // reserved
	tkhd = append(tkhd, identityMatrix()...)        // matrix
	tkhd = append(tkhd, u32(uint32(width)<<16)...)  // width 16.16
	tkhd = append(tkhd, u32(uint32(height)<<16)...) // height 16.16

	trak := box("trak", box("tkhd", tkhd))
	moov := box("moov", append(box("mvhd", mvhd), trak...))
	ftyp := box("ftyp", append([]byte("isom"), append(u32(0), []byte("isom")...)...))
	mdat := make([]byte, 8+filler)
	binary.BigEndian.PutUint32(mdat[0:4], uint32(8+filler))
	copy(mdat[4:8], "mdat")
	for i := 8; i < len(mdat); i++ {
		mdat[i] = byte(i % 251)
	}
	return append(append(ftyp, moov...), mdat...)
}

func box(typ string, payload []byte) []byte {
	out := make([]byte, 8+len(payload))
	binary.BigEndian.PutUint32(out[0:4], uint32(len(out)))
	copy(out[4:8], typ)
	copy(out[8:], payload)
	return out
}

func u32(v uint32) []byte {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], v)
	return b[:]
}

func identityMatrix() []byte {
	var m []byte
	m = append(m, u32(0x00010000)...)
	m = append(m, u32(0)...)
	m = append(m, u32(0)...)
	m = append(m, u32(0)...)
	m = append(m, u32(0x00010000)...)
	m = append(m, u32(0)...)
	m = append(m, u32(0)...)
	m = append(m, u32(0)...)
	m = append(m, u32(0x40000000)...)
	return m
}

// buildMinimalWebM constructs: EBML header + Segment{Info{TimestampScale,
// Duration}}. The Segment uses an unknown size, as real WebM files often do.
func buildMinimalWebM(durationMs float64) []byte {
	// Info children.
	ts := encodeEBMLUint(ebmlIDTimestampScale, 1_000_000) // 1 ms scale
	dur := encodeEBMLFloat(ebmlIDDuration, float32(durationMs))
	info := ebmlElement(ebmlIDInfo, append(ts, dur...))

	// EBML header.
	docType := encodeEBMLString(0x4282, "webm")
	ver := encodeEBMLUint(0x4287, 2)
	readVer := encodeEBMLUint(0x4285, 2)
	ebmlVer := encodeEBMLUint(0x4286, 1)
	ebmlReadVer := encodeEBMLUint(0x42F7, 1)
	maxID := encodeEBMLUint(0x42F2, 4)
	maxSize := encodeEBMLUint(0x42F3, 8)
	header := ebmlElement(0x1A45DFA3, concat(ebmlVer, ebmlReadVer, maxID, maxSize, docType, ver, readVer))

	// Segment with unknown (all-ones 8-byte) size.
	segHeader := append(ebmlIDBytes(ebmlIDSegment), []byte{0x01, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}...)
	return append(header, append(segHeader, info...)...)
}

func concat(parts ...[]byte) []byte {
	var out []byte
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

func ebmlIDBytes(id uint64) []byte {
	switch {
	case id > 0xFFFFFF:
		return []byte{byte(id >> 24), byte(id >> 16), byte(id >> 8), byte(id)}
	case id > 0xFFFF:
		return []byte{byte(id >> 16), byte(id >> 8), byte(id)}
	case id > 0xFF:
		return []byte{byte(id >> 8), byte(id)}
	default:
		return []byte{byte(id)}
	}
}

// encodeEBMLSizeVint encodes a data-size vint in minimal length.
func encodeEBMLSizeVint(v uint64) []byte {
	for n := 1; n <= 8; n++ {
		bits := 7 * n
		if n == 8 {
			bits = 56
		}
		maxVal := uint64(1)<<uint(bits) - 1
		if v <= maxVal-1 { // reserve all-ones for "unknown"
			out := make([]byte, n)
			val := v
			for i := n - 1; i > 0; i-- {
				out[i] = byte(val & 0xFF)
				val >>= 8
			}
			out[0] = byte(val) | byte(1<<(8-n)) // set marker bit
			return out
		}
	}
	return []byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}
}

func encodeEBMLUint(id, v uint64) []byte {
	var val []byte
	switch {
	case v > 0xFFFFFF:
		val = []byte{byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)}
	case v > 0xFFFF:
		val = []byte{byte(v >> 16), byte(v >> 8), byte(v)}
	case v > 0xFF:
		val = []byte{byte(v >> 8), byte(v)}
	default:
		val = []byte{byte(v)}
	}
	return append(ebmlIDBytes(id), append(encodeEBMLSizeVint(uint64(len(val))), val...)...)
}

func encodeEBMLString(id uint64, s string) []byte {
	v := []byte(s)
	return append(ebmlIDBytes(id), append(encodeEBMLSizeVint(uint64(len(v))), v...)...)
}

func encodeEBMLFloat(id uint64, f float32) []byte {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], math.Float32bits(f))
	return append(ebmlIDBytes(id), append(encodeEBMLSizeVint(4), b[:]...)...)
}

// ebmlElement encodes id + size-vint + payload.
func ebmlElement(id uint64, payload []byte) []byte {
	return append(ebmlIDBytes(id), append(encodeEBMLSizeVint(uint64(len(payload))), payload...)...)
}
