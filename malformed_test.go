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

func TestGridTemplateLengthBoundaries(t *testing.T) {
	// Canonical template-body sizes from the WMO/ecCodes definitions.
	tests := []struct {
		name     string
		template uint16
		body     int
	}{
		{"latlon", 0, 58},
		{"rotated-latlon", 1, 70},
		{"mercator", 10, 58},
		{"polar", 20, 51},
		{"lambert", 30, 67},
		{"gaussian", 40, 58},
		{"rotated-gaussian", 41, 70},
		{"stretched-gaussian", 42, 70},
		{"stretched-rotated-gaussian", 43, 82},
		{"spectral", 50, 14},
		{"unstructured", 101, 21},
	}
	section3 := func(template uint16, body int) section.Section3 {
		raw := make([]byte, 14+body)
		binary.BigEndian.PutUint32(raw, uint32(len(raw)))
		raw[4] = 3
		binary.BigEndian.PutUint32(raw[6:], 1)
		binary.BigEndian.PutUint16(raw[12:], template)
		if body > 0 {
			raw[14] = 6
		}
		return section.Section3{Raw: raw}
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := &Message{S3: section3(tc.template, tc.body)}
			if _, err := m.Grid(); err != nil {
				t.Fatalf("exact %d-byte template: %v", tc.body, err)
			}
			m = &Message{S3: section3(tc.template, tc.body-1)}
			if _, err := m.Grid(); !errors.Is(err, ErrTruncated) {
				t.Errorf("%d-byte template error = %v, want ErrTruncated", tc.body-1, err)
			}
		})
	}
}
