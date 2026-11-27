package main

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	// Register decoders for image.Decode/DecodeConfig.
	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"
	_ "image/gif"
	_ "image/png"

	"golang.org/x/image/draw"
)

// jpegQuality matches the contract's "q≈80".
const jpegQuality = 80

// generateThumb produces the cached JPEG for an item, or leaves the cache
// empty on failure (the handler then serves the placeholder).
func (app *App) generateThumb(it *itemRow, variant string, cachePath string) {
	edge := app.cfg.ThumbFullEdge
	if variant == "grid" {
		edge = app.cfg.ThumbGridEdge
	}

	var src image.Image
	switch {
	case it.Type == "photo":
		img, err := decodeImageFile(it.Path)
		if err != nil {
			return // placeholder
		}
		src = img
	default: // video: poster frame via ffmpeg when available
		img, err := app.ffmpegPosterFrame(it.Path)
		if err != nil {
			return // placeholder
		}
		src = img
	}

	out := scaleToLongEdge(src, edge)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, out, &jpeg.Options{Quality: jpegQuality}); err != nil {
		return
	}
	writeFileAtomic(cachePath, buf.Bytes(), 0o600)
}

// decodeImageFile fully decodes a photo (any registered format).
func decodeImageFile(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	return img, err
}

// ffmpegPosterFrame extracts the first frame of a video as a JPEG via ffmpeg,
// when ffmpeg is available (config override or PATH lookup).
func (app *App) ffmpegPosterFrame(videoPath string) (image.Image, error) {
	ff := app.cfg.FFmpegPath
	if ff == "" {
		var err error
		ff, err = exec.LookPath("ffmpeg")
		if err != nil {
			return nil, err
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, ff,
		"-hide_banner", "-loglevel", "error",
		"-ss", "0", "-i", videoPath,
		"-frames:v", "1", "-q:v", "2",
		"-f", "mjpeg", "pipe:1")
	var out, errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	img, err := jpeg.Decode(&out)
	if err != nil {
		return nil, err
	}
	_ = errOut
	return img, nil
}

// scaleToLongEdge resizes so the long edge is at most edge px, never
// upscaling, preserving aspect ratio.
func scaleToLongEdge(src image.Image, edge int) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return src
	}
	tw, th := w, h
	if w >= h {
		if w > edge {
			tw = edge
			th = max(1, (h*edge+w-1)/w)
		}
	} else {
		if h > edge {
			th = edge
			tw = max(1, (w*edge+h-1)/h)
		}
	}
	if tw == w && th == h {
		return src
	}
	dst := image.NewRGBA(image.Rect(0, 0, tw, th))
	draw.BiLinear.Scale(dst, dst.Bounds(), src, b, draw.Over, nil)
	return dst
}

// placeholderJPEG renders a small dark gradient JPEG. 16:9, long edge = edge.
func placeholderJPEG(edge int) []byte {
	w, h := edge, (edge*9+15)/16
	if w < 2 {
		w = 2
	}
	if h < 2 {
		h = 2
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	top := color.RGBA{R: 0x14, G: 0x16, B: 0x1a, A: 0xff}
	bottom := color.RGBA{R: 0x1d, G: 0x21, B: 0x28, A: 0xff}
	for y := 0; y < h; y++ {
		t := float64(y) / float64(max(1, h-1))
		c := color.RGBA{
			R: lerp(top.R, bottom.R, t),
			G: lerp(top.G, bottom.G, t),
			B: lerp(top.B, bottom.B, t),
			A: 0xff,
		}
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	_ = jpeg.Encode(&buf, img, &jpeg.Options{Quality: jpegQuality})
	return buf.Bytes()
}

func lerp(a, b uint8, t float64) uint8 {
	return uint8(float64(a) + (float64(b)-float64(a))*t)
}

// writeFileAtomic writes bytes to path via a temp file + rename so readers
// never observe a partially written thumbnail.
func writeFileAtomic(path string, data []byte, mode os.FileMode) {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".thumb-*")
	if err != nil {
		return
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		os.Remove(tmpName)
		return
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
	}
}

// --- keyed singleflight -------------------------------------------------------
//
// keyGate serializes work per key: the first caller runs fn, everyone else
// waits and then reads the (now cached) result.

type keyGate struct {
	mu    sync.Mutex
	slots map[string]chan struct{}
}

func newKeyGate() *keyGate {
	return &keyGate{slots: make(map[string]chan struct{})}
}

// do runs fn for key, or waits for a concurrent run to finish first.
func (g *keyGate) do(key string, fn func()) {
	g.mu.Lock()
	if done, ok := g.slots[key]; ok {
		g.mu.Unlock()
		<-done // wait for the in-flight generation
		return
	}
	done := make(chan struct{})
	g.slots[key] = done
	g.mu.Unlock()
	defer func() {
		g.mu.Lock()
		delete(g.slots, key)
		close(done)
		g.mu.Unlock()
	}()
	fn()
}

// removeThumbs deletes both cached variants for an item (best effort) — used
// when an item changes or disappears.
func (app *App) removeThumbs(itemID int64) {
	for _, v := range []string{"grid", "full"} {
		_ = os.Remove(app.thumbCachePath(itemID, v))
	}
}
