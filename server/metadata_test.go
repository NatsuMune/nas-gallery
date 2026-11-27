package main

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestMP4MetadataParsers(t *testing.T) {
	// Version 0 mvhd + tkhd via minimal MP4 fixture generator.
	raw := buildMinimalMP4(30400, 1920, 1080, 64)
	info, err := parseMP4Info(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("parseMP4Info v0 failed: %v", err)
	}
	if info.DurationMs != 30400 || info.Width != 1920 || info.Height != 1080 {
		t.Fatalf("unexpected v0 mp4 metadata: %+v", info)
	}

	// Version 1 mvhd (64-bit creation/modification/duration).
	var mvhdV1 []byte
	mvhdV1 = append(mvhdV1, 1, 0, 0, 0) // version 1
	mvhdV1 = append(mvhdV1, make([]byte, 16)...)
	mvhdV1 = append(mvhdV1, u32(1000)...) // timescale 1000
	var dur64 [8]byte
	binary.BigEndian.PutUint64(dur64[:], 90_000) // 90s
	mvhdV1 = append(mvhdV1, dur64[:]...)
	moov := box("moov", box("mvhd", mvhdV1))
	ftyp := box("ftyp", []byte("isom\x00\x00\x00\x00isom"))
	v1Raw := append(ftyp, moov...)

	infoV1, err := parseMP4Info(bytes.NewReader(v1Raw))
	if err != nil {
		t.Fatalf("parseMP4Info v1 failed: %v", err)
	}
	if infoV1.DurationMs != 90000 {
		t.Fatalf("expected 90000ms for v1 mvhd, got %d", infoV1.DurationMs)
	}

	// Garbage input fails cleanly.
	if _, err := parseMP4Info(bytes.NewReader([]byte("not an mp4"))); err == nil {
		t.Fatalf("expected error on garbage mp4")
	}
}

func TestWebMEBMLDurationParser(t *testing.T) {
	raw := buildMinimalWebM(42500)
	ms, err := parseMatroskaDuration(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("parseMatroskaDuration failed: %v", err)
	}
	if ms != 42500 {
		t.Fatalf("expected 42500ms, got %d", ms)
	}

	// Handcrafted EBML with custom TimestampScale (500_000 ns = 0.5 ms) and 32-bit float Duration (200.0 -> 100 ms).
	infoBody := append(
		encodeEBMLUint(ebmlIDTimestampScale, 500_000),
		encodeEBMLFloat(ebmlIDDuration, 200.0)...,
	)
	seg := ebmlElement(ebmlIDSegment, ebmlElement(ebmlIDInfo, infoBody))
	ebmlHdr := ebmlElement(0x1A45DFA3, []byte{0x42, 0x86, 0x81, 0x01})
	custom := append(ebmlHdr, seg...)

	ms2, err := parseMatroskaDuration(bytes.NewReader(custom))
	if err != nil {
		t.Fatalf("custom scale webm failed: %v", err)
	}
	if ms2 != 100 {
		t.Fatalf("expected 100ms, got %d", ms2)
	}

	// Garbage input fails cleanly.
	if _, err := parseMatroskaDuration(bytes.NewReader([]byte("nope"))); err == nil {
		t.Fatalf("expected error on invalid EBML")
	}
}
