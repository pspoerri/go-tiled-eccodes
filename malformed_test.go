package grib

import (
	"encoding/binary"
	"errors"
	"testing"

	"github.com/pspoerri/go-tiled-eccodes/section"
)

func malformedMessageWithSections(sections ...[]byte) []byte {
	length := 16 + 4
	for _, s := range sections {
		length += len(s)
	}
	out := make([]byte, 0, length)
	s0 := make([]byte, 16)
	copy(s0, "GRIB")
	s0[7] = 2
	binary.BigEndian.PutUint64(s0[8:], uint64(length))
	out = append(out, s0...)
	for _, s := range sections {
		out = append(out, s...)
	}
	return append(out, '7', '7', '7', '7')
}

func TestIndexRejectsMissingAndShortSections(t *testing.T) {
	noFields := malformedMessageWithSections()
	if _, err := Index(noFields); !errors.Is(err, ErrBadSection) {
		t.Errorf("no-fields error = %v, want ErrBadSection", err)
	}

	shortSection1 := []byte{0, 0, 0, 5, 1}
	if _, err := Index(malformedMessageWithSections(shortSection1)); !errors.Is(err, ErrBadSection) {
		t.Errorf("short-section error = %v, want ErrBadSection", err)
	}

	section7 := []byte{0, 0, 0, 5, 7}
	if _, err := Index(malformedMessageWithSections(section7)); !errors.Is(err, ErrBadSection) {
		t.Errorf("missing-mandatory error = %v, want ErrBadSection", err)
	}
}

func TestGridRejectsShortSupportedTemplate(t *testing.T) {
	raw := make([]byte, 14)
	binary.BigEndian.PutUint16(raw[12:], 0)
	m := &Message{S3: section.Section3{Raw: raw}}
	if _, err := m.Grid(); !errors.Is(err, ErrTruncated) {
		t.Errorf("Grid error = %v, want ErrTruncated", err)
	}
}
