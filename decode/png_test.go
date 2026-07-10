package decode

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func TestPNGRGBAndRGBA(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	img.SetNRGBA(0, 0, color.NRGBA{R: 0x01, G: 0x02, B: 0x03, A: 0x04})
	img.SetNRGBA(1, 0, color.NRGBA{R: 0xa0, G: 0xb0, B: 0xc0, A: 0xd0})

	var encoded bytes.Buffer
	if err := png.Encode(&encoded, img); err != nil {
		t.Fatalf("encode PNG: %v", err)
	}

	for _, tc := range []struct {
		name  string
		nbits byte
		want  []float64
	}{
		{name: "RGB24", nbits: 24, want: []float64{0x010203, 0xa0b0c0}},
		{name: "RGBA32", nbits: 32, want: []float64{0x01020304, 0xa0b0c0d0}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			template := make([]byte, 10)
			template[8] = tc.nbits
			got, err := PNG(template, encoded.Bytes(), 2, nil)
			if err != nil {
				t.Fatalf("PNG: %v", err)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Errorf("value[%d] = %#x, want %#x", i, uint64(got[i]), uint64(tc.want[i]))
				}
			}
		})
	}
}
