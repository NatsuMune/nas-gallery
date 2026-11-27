package main

import (
	"bufio"
	"image"
	"os"
)

// extractItemMeta returns best-effort metadata for a media file:
//   - photos: dimensions via image.DecodeConfig (jpg/png/gif/webp/bmp/tiff
//     registered; avif/heic etc. fail gracefully → nil)
//   - videos: duration via native MP4-family (moov/mvhd) or EBML/Matroska
//     (Segment/Info/Duration) parsing; dimensions best-effort from MP4 tkhd
//
// Missing values are 0, which the scanner stores as SQL NULL.
func extractItemMeta(path, ext, mediaType string) (width, height, durationMs int) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, 0
	}
	defer f.Close()
	if mediaType == "photo" {
		w, h, err := photoDimensions(f)
		if err != nil {
			return 0, 0, 0
		}
		return w, h, 0
	}
	switch {
	case mp4FamilyExts[ext]:
		info, err := parseMP4Info(f)
		if err != nil {
			return 0, 0, 0
		}
		return info.Width, info.Height, info.DurationMs
	case matroskaFamilyExts[ext]:
		dur, err := parseMatroskaDuration(f)
		if err != nil {
			return 0, 0, 0
		}
		return 0, 0, dur
	}
	return 0, 0, 0
}

// photoDimensions reads image header bytes only (image.DecodeConfig wraps the
// reader in a bufio.Reader internally).
func photoDimensions(f *os.File) (int, int, error) {
	cfg, _, err := image.DecodeConfig(bufio.NewReader(f))
	if err != nil {
		return 0, 0, err
	}
	return cfg.Width, cfg.Height, nil
}
