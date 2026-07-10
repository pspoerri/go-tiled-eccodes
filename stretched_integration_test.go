package grib_test

import (
	"encoding/binary"
	"math"
	"testing"

	grib "github.com/pspoerri/go-tiled-eccodes"
	"github.com/pspoerri/go-tiled-eccodes/writer"
)

type testStretchedGrid struct {
	number uint16
	body   []byte
}

func (g testStretchedGrid) TemplateNumber() uint16  { return g.number }
func (g testStretchedGrid) EncodeTemplate() []byte  { return append([]byte(nil), g.body...) }
func (g testStretchedGrid) NumPoints() int          { return 32 }
func (g testStretchedGrid) NaturalSize() (int, int) { return 8, 4 }
func (g testStretchedGrid) StorageIndex(i, j int) int {
	if i < 0 || i >= 8 || j < 0 || j >= 4 {
		return -1
	}
	return j*8 + i
}

func stretchedTemplate(rotated bool) []byte {
	n := 70
	if rotated {
		n = 82
	}
	body := make([]byte, n)
	body[0] = 6
	binary.BigEndian.PutUint32(body[16:], 8)
	binary.BigEndian.PutUint32(body[20:], 4)
	binary.BigEndian.PutUint32(body[28:], 0xffffffff)
	putSM32(body[32:], 60_000_000)
	putSM32(body[36:], 0)
	putSM32(body[41:], -60_000_000)
	putSM32(body[45:], 315_000_000)
	binary.BigEndian.PutUint32(body[49:], 45_000_000)
	binary.BigEndian.PutUint32(body[53:], 4)
	if rotated {
		putSM32(body[58:], -90_000_000)
		binary.BigEndian.PutUint32(body[62:], 0)
		binary.BigEndian.PutUint32(body[66:], math.Float32bits(0))
		putSM32(body[70:], 90_000_000)
		putSM32(body[74:], 0)
		binary.BigEndian.PutUint32(body[78:], 1_000_000)
	} else {
		putSM32(body[58:], 90_000_000)
		putSM32(body[62:], 0)
		binary.BigEndian.PutUint32(body[66:], 1_000_000)
	}
	return body
}

func putSM32(dst []byte, value int32) {
	magnitude := uint32(value)
	if value < 0 {
		magnitude = uint32(-int64(value)) | 0x80000000
	}
	binary.BigEndian.PutUint32(dst, magnitude)
}

func TestStretchedGaussianEndToEnd(t *testing.T) {
	for _, number := range []uint16{42, 43} {
		t.Run(string(rune(number)), func(t *testing.T) {
			grid := testStretchedGrid{number: number, body: stretchedTemplate(number == 43)}
			values := make([]float64, grid.NumPoints())
			for i := range values {
				values[i] = float64(i)
			}
			data, err := writer.Single(minimalField(grid, values))
			if err != nil {
				t.Fatalf("Single: %v", err)
			}
			file, err := grib.FromBytes(data)
			if err != nil {
				t.Fatalf("FromBytes: %v", err)
			}
			defer file.Close()
			if _, err := file.Messages()[0].Grid(); err != nil {
				t.Fatalf("Grid: %v", err)
			}
			got, err := file.Messages()[0].DecodeFloat64(nil)
			if err != nil {
				t.Fatalf("DecodeFloat64: %v", err)
			}
			if len(got) != len(values) {
				t.Fatalf("decoded len = %d, want %d", len(got), len(values))
			}
		})
	}
}
