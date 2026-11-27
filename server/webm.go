package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
)

// EBML element IDs used by the Matroska/WebM duration parser.
const (
	ebmlIDSegment        = 0x18538067
	ebmlIDInfo           = 0x1549A966
	ebmlIDTimestampScale = 0x2AD7B1
	ebmlIDDuration       = 0x4489
)

// defaultTimestampScale is 1 ms in nanoseconds when the element is absent.
const defaultTimestampScale = 1_000_000

// parseMatroskaDuration reads the Segment → Info → Duration (and
// TimestampScale) elements of a WebM/Matroska file and returns the duration in
// milliseconds. Only headers are read; no frame data is touched.
func parseMatroskaDuration(r io.ReadSeeker) (int, error) {
	// Skip the EBML header element.
	id, size, hdrLen, err := readEBMLElementHeader(r, 0)
	if err != nil {
		return 0, fmt.Errorf("ebml: read header: %w", err)
	}
	// Tolerate files without the EBML header but reject clearly wrong data.
	if id != 0x1A45DFA3 && id != ebmlIDSegment {
		return 0, fmt.Errorf("ebml: unexpected first element %#x", id)
	}
	pos := int64(hdrLen)
	if id == 0x1A45DFA3 {
		pos += int64(size)
	}

	// Segment element.
	id, size, hdrLen, err = readEBMLElementHeader(r, pos)
	if err != nil {
		return 0, fmt.Errorf("ebml: read segment: %w", err)
	}
	if id != ebmlIDSegment {
		return 0, fmt.Errorf("ebml: expected Segment, got %#x", id)
	}
	segPayload := size
	if segPayload == 0 { // unknown size: payload is the rest of the file
		end, err := r.Seek(0, io.SeekEnd)
		if err != nil {
			return 0, err
		}
		segPayload = uint64(end - pos - int64(hdrLen))
	}
	segEnd := pos + int64(hdrLen) + int64(segPayload)
	pos += int64(hdrLen)

	// Scan Segment children for Info.
	for pos < segEnd {
		id, size, hdrLen, err := readEBMLElementHeader(r, pos)
		if err != nil {
			return 0, fmt.Errorf("ebml: scan segment: %w", err)
		}
		if size == 0 { // unknown-size child: cannot skip reliably
			return 0, fmt.Errorf("ebml: unknown-size child inside Segment")
		}
		if id == ebmlIDInfo {
			body := make([]byte, size)
			if _, err := readAt(r, pos+int64(hdrLen), body); err != nil && err != io.EOF {
				return 0, fmt.Errorf("ebml: read info: %w", err)
			}
			return parseInfoDuration(body)
		}
		pos += int64(hdrLen) + int64(size)
	}
	return 0, fmt.Errorf("ebml: no Info element found")
}

// parseInfoDuration reads TimestampScale (uint) and Duration (float) children.
func parseInfoDuration(body []byte) (int, error) {
	scale := uint64(defaultTimestampScale)
	duration := math.NaN()
	pos := 0
	for pos+1 < len(body) {
		id, idLen, err := readEBMLID(body[pos:])
		if err != nil {
			break
		}
		pos += idLen
		size, sizeLen, unknown, err := readEBMLSize(body[pos:])
		if err != nil {
			break
		}
		pos += sizeLen
		if unknown || pos+int(size) > len(body) {
			break
		}
		val := body[pos : pos+int(size)]
		switch id {
		case ebmlIDTimestampScale:
			if n := ebmlUint(val); n > 0 {
				scale = n
			}
		case ebmlIDDuration:
			if duration, err = ebmlFloat(val); err != nil {
				duration = math.NaN()
			}
		}
		pos += int(size)
	}
	if math.IsNaN(duration) {
		return 0, fmt.Errorf("ebml: no Duration element")
	}
	// Duration (unitless, in TimestampScale units) × scale (ns) → nanoseconds.
	ns := duration * float64(scale)
	if ns < 0 || ns > 1e15 { // sanity: > ~31 years means parse garbage
		return 0, fmt.Errorf("ebml: implausible duration %v ns", ns)
	}
	return int((ns + 500_000) / 1_000_000), nil // round to nearest ms
}

// readEBMLElementHeader reads an element ID + data size vint at pos.
// size == 0 means "unknown size" (all-ones size vint).
func readEBMLElementHeader(r io.ReadSeeker, pos int64) (id uint64, size uint64, hdrLen int, err error) {
	if _, err := r.Seek(pos, io.SeekStart); err != nil {
		return 0, 0, 0, err
	}
	var idBuf [8]byte
	n, err := io.ReadFull(r, idBuf[:1])
	if err != nil || n < 1 {
		return 0, 0, 0, io.EOF
	}
	idLen := vintLength(idBuf[0])
	if _, err := io.ReadFull(r, idBuf[1:idLen]); err != nil {
		return 0, 0, 0, fmt.Errorf("ebml: truncated element id")
	}
	id = uint64(0)
	for _, b := range idBuf[:idLen] {
		id = id<<8 | uint64(b)
	}
	var sizeBuf [8]byte
	if _, err := io.ReadFull(r, sizeBuf[:1]); err != nil {
		return 0, 0, 0, fmt.Errorf("ebml: truncated element size")
	}
	sizeLen := vintLength(sizeBuf[0])
	if _, err := io.ReadFull(r, sizeBuf[1:sizeLen]); err != nil {
		return 0, 0, 0, fmt.Errorf("ebml: truncated element size")
	}
	size, unknown := decodeSizeVint(sizeBuf[:sizeLen])
	if unknown {
		return id, 0, idLen + sizeLen, nil
	}
	return id, size, idLen + sizeLen, nil
}

// readEBMLID parses an element ID from the front of b; returns the ID value
// (marker bit included) and its encoded length.
func readEBMLID(b []byte) (uint64, int, error) {
	if len(b) == 0 {
		return 0, 0, io.EOF
	}
	n := vintLength(b[0])
	if len(b) < n {
		return 0, 0, fmt.Errorf("ebml: truncated id")
	}
	id := uint64(0)
	for _, bb := range b[:n] {
		id = id<<8 | uint64(bb)
	}
	return id, n, nil
}

// readEBMLSize parses a data-size vint; unknown reports all-ones (unknown).
func readEBMLSize(b []byte) (size uint64, n int, unknown bool, err error) {
	if len(b) == 0 {
		return 0, 0, false, io.EOF
	}
	n = vintLength(b[0])
	if len(b) < n {
		return 0, 0, false, fmt.Errorf("ebml: truncated size")
	}
	size, unknown = decodeSizeVint(b[:n])
	return size, n, unknown, nil
}

// vintLength returns the encoded length (1..8) implied by the first byte of a
// vint (leading-zero count + 1).
func vintLength(first byte) int {
	for i := 0; i < 8; i++ {
		if first&(1<<(7-i)) != 0 {
			return i + 1
		}
	}
	return 8 // first == 0: reserved, treat as 8
}

// decodeSizeVint decodes a size vint, stripping the marker bit; unknown means
// all value bits are 1.
func decodeSizeVint(b []byte) (uint64, bool) {
	n := len(b)
	if n == 0 || n > 8 {
		return 0, false
	}
	v := uint64(b[0]) & (0xFF >> uint(n)) // strip marker bit(s) of first byte
	for _, bb := range b[1:] {
		v = v<<8 | uint64(bb)
	}
	allOnes := v == (uint64(1)<<(7*uint(n)))-1
	return v, allOnes
}

func ebmlUint(b []byte) uint64 {
	var v uint64
	for _, bb := range b {
		v = v<<8 | uint64(bb)
	}
	return v
}

func ebmlFloat(b []byte) (float64, error) {
	switch len(b) {
	case 4:
		return float64(math.Float32frombits(binary.BigEndian.Uint32(b))), nil
	case 8:
		return math.Float64frombits(binary.BigEndian.Uint64(b)), nil
	default:
		return 0, fmt.Errorf("ebml: unsupported float length %d", len(b))
	}
}
