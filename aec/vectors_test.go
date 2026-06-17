package aec

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"strconv"
	"testing"
)

type vector struct {
	Name          string `json:"name"`
	BitsPerSample int    `json:"bitsPerSample"`
	BlockSize     int    `json:"blockSize"`
	RSI           int    `json:"rsi"`
	Flags         uint   `json:"flags"`
	SamplesB64    string `json:"samplesBase64"` // expected decoded bytes
	StreamB64     string `json:"streamBase64"`  // AEC input bitstream
}

func loadVectors(t testing.TB) []vector {
	t.Helper()
	raw, err := os.ReadFile("testdata/vectors.json")
	if err != nil {
		t.Fatalf("read fixtures: %v", err)
	}
	var vs []vector
	if err := json.Unmarshal(raw, &vs); err != nil {
		t.Fatalf("parse fixtures: %v", err)
	}
	if len(vs) == 0 {
		t.Fatal("no fixtures")
	}
	return vs
}

// TestGoldenVectors decodes every frozen vector and checks byte-exact output.
func TestGoldenVectors(t *testing.T) {
	for _, v := range loadVectors(t) {
		t.Run(v.Name, func(t *testing.T) {
			want, _ := base64.StdEncoding.DecodeString(v.SamplesB64)
			stream, _ := base64.StdEncoding.DecodeString(v.StreamB64)
			dst := make([]byte, len(want))
			n, err := Decode(dst, stream, Config{
				BitsPerSample: v.BitsPerSample, BlockSize: v.BlockSize,
				RSI: v.RSI, Flags: Flags(v.Flags),
			})
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			if n != len(want) || string(dst[:n]) != string(want) {
				t.Fatalf("mismatch: n=%d want %d bytes", n, len(want))
			}
		})
	}
}

func maskBits(bps int) uint32 {
	if bps >= 32 {
		return ^uint32(0)
	}
	return uint32(1)<<uint(bps) - 1
}

func bytesPerSampleFor(bps int, flags Flags) int {
	switch {
	case bps <= 8:
		return 1
	case bps <= 16:
		return 2
	case bps <= 24 && flags&Data3Byte != 0:
		return 3
	default:
		return 4
	}
}

func writeSample(buf []byte, i int, v uint32, bytesPer int, msb bool) {
	o := i * bytesPer
	for b := 0; b < bytesPer; b++ {
		if msb {
			buf[o+b] = byte(v >> uint(8*(bytesPer-1-b)))
		} else {
			buf[o+b] = byte(v >> uint(8*b))
		}
	}
}

func vecName(bps, bs, rsi int, fl Flags, kind string) string {
	return kind + "_bps" + itoa(bps) + "_bs" + itoa(bs) + "_rsi" + itoa(rsi) + "_fl" + itoa(int(fl))
}

func itoa(n int) string { return strconv.Itoa(n) }
