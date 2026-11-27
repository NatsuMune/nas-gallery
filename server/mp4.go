package main

import (
	"encoding/binary"
	"fmt"
	"io"
)

// mp4Info is the metadata extracted from a QuickTime-family container
// (mp4/m4v/mov): movie duration from moov/mvhd and presentation dimensions
// from the first usable moov/trak/tkhd (16.16 fixed-point).
type mp4Info struct {
	DurationMs int
	Width      int
	Height     int
}

// maxMoovSize caps how much of a moov box we are willing to read into memory
// (normal moov boxes are a few KiB; this guards against corrupt files).
const maxMoovSize = 64 << 20

// parseMP4Info walks the top-level boxes of a QuickTime-family file.
func parseMP4Info(r io.ReadSeeker) (mp4Info, error) {
	var info mp4Info
	pos := int64(0)
	for {
		size, typ, hdrLen, err := readBoxHeader(r, pos)
		if err != nil {
			break // EOF or truncation: parse what we have
		}
		payload := size - int64(hdrLen)
		if payload < 0 {
			return info, fmt.Errorf("mp4: malformed box %q at %d", typ, pos)
		}
		if typ == "moov" {
			if payload > maxMoovSize {
				return info, fmt.Errorf("mp4: moov too large (%d bytes)", payload)
			}
			body := make([]byte, payload)
			if _, err := readAt(r, pos+int64(hdrLen), body); err != nil && err != io.EOF {
				return info, fmt.Errorf("mp4: read moov: %w", err)
			}
			if err := parseMoov(body, &info); err != nil {
				return info, err
			}
		}
		if size == 0 { // box extends to EOF
			break
		}
		pos += size
	}
	if !info.hasAny() {
		return info, fmt.Errorf("mp4: no moov box found")
	}
	return info, nil
}

func (i mp4Info) hasAny() bool { return i.DurationMs > 0 || i.Width > 0 || i.Height > 0 }

// readBoxHeader reads an 8-byte (or 16-byte extended) box header at pos.
// It returns the total box size, the 4-char type and the header length.
func readBoxHeader(r io.ReadSeeker, pos int64) (size int64, typ string, hdrLen int, err error) {
	var head [8]byte
	n, err := readAt(r, pos, head[:])
	if err != nil && err != io.EOF {
		return 0, "", 0, err
	}
	if n < 8 {
		return 0, "", 0, io.EOF
	}
	typ = string(head[4:8])
	size = int64(binary.BigEndian.Uint32(head[0:4]))
	hdrLen = 8
	if size == 1 { // 64-bit extended size
		var ext [8]byte
		n, err := readAt(r, pos+8, ext[:])
		if err != nil || n < 8 {
			return 0, "", 0, fmt.Errorf("mp4: truncated extended size")
		}
		size = int64(binary.BigEndian.Uint64(ext[:]))
		hdrLen = 16
	}
	return size, typ, hdrLen, nil
}

// readAt reads exactly len(b) bytes at absolute offset off, returning the
// number of bytes read.
func readAt(r io.ReadSeeker, off int64, b []byte) (int, error) {
	if _, err := r.Seek(off, io.SeekStart); err != nil {
		return 0, err
	}
	return io.ReadFull(r, b)
}

// parseMoov parses boxes inside the moov payload.
func parseMoov(body []byte, info *mp4Info) error {
	pos := 0
	for pos+8 <= len(body) {
		size := int64(binary.BigEndian.Uint32(body[pos : pos+4]))
		typ := string(body[pos+4 : pos+8])
		hdrLen := 8
		if size == 1 && pos+16 <= len(body) {
			size = int64(binary.BigEndian.Uint64(body[pos+8 : pos+16]))
			hdrLen = 16
		}
		if size < int64(hdrLen) || int64(pos)+size > int64(len(body)) {
			return fmt.Errorf("mp4: malformed moov child %q", typ)
		}
		payload := body[pos+hdrLen : pos+int(size)]
		switch typ {
		case "mvhd":
			parseMvhd(payload, info)
		case "trak":
			parseTrak(payload, info)
		}
		if size == 0 {
			break
		}
		pos += int(size)
	}
	return nil
}

// parseMvhd reads the movie timescale + duration (version 0 or 1).
func parseMvhd(p []byte, info *mp4Info) {
	if len(p) < 4 {
		return
	}
	version := p[0]
	switch version {
	case 0:
		if len(p) < 20 {
			return
		}
		timescale := binary.BigEndian.Uint32(p[12:16])
		duration := binary.BigEndian.Uint32(p[16:20])
		if timescale > 0 {
			info.DurationMs = int(uint64(duration) * 1000 / uint64(timescale))
		}
	case 1:
		if len(p) < 32 {
			return
		}
		timescale := binary.BigEndian.Uint32(p[20:24])
		duration := binary.BigEndian.Uint64(p[24:32])
		if timescale > 0 {
			info.DurationMs = int(duration * 1000 / uint64(timescale))
		}
	}
}

// parseTrak looks for a tkhd with nonzero width/height (the video track).
func parseTrak(p []byte, info *mp4Info) {
	pos := 0
	for pos+8 <= len(p) {
		size := int64(binary.BigEndian.Uint32(p[pos : pos+4]))
		typ := string(p[pos+4 : pos+8])
		if size < 8 || pos+int(size) > len(p) {
			return
		}
		if typ == "tkhd" {
			parseTkhd(p[pos+8:pos+int(size)], info)
			// keep scanning further traks only if nothing usable yet
			if info.Width > 0 && info.Height > 0 {
				return
			}
		}
		if size == 0 {
			return
		}
		pos += int(size)
	}
}

// parseTkhd reads the 16.16 fixed-point width/height. Layout (version 0):
// 4 ver/flags, 4 created, 4 modified, 4 trackID, 4 reserved, 4 duration,
// 8 reserved, 2 layer, 2 altGroup, 2 volume, 2 reserved, 36 matrix, 4 width, 4 height.
func parseTkhd(p []byte, info *mp4Info) {
	if len(p) < 4 {
		return
	}
	version := p[0]
	base := 0
	switch version {
	case 0:
		base = 4 + 20 // ver/flags + created + modified + trackID + reserved + duration
	case 1:
		base = 4 + 32 // ver/flags + created(8) + modified(8) + trackID(4) + reserved(4) + duration(8)
	default:
		return
	}
	off := base + 8 /* reserved */ + 2 /* layer */ + 2 /* altGroup */ + 2 /* volume */ + 2 /* reserved */ + 36 /* matrix */
	if off+8 > len(p) {
		return
	}
	w := fixed1616(binary.BigEndian.Uint32(p[off : off+4]))
	h := fixed1616(binary.BigEndian.Uint32(p[off+4 : off+8]))
	if w > 0 {
		info.Width = w
	}
	if h > 0 {
		info.Height = h
	}
}

// fixed1616 converts a 16.16 fixed-point value to an int (rounded).
func fixed1616(v uint32) int {
	return int((uint64(v) + 0x8000) >> 16)
}
