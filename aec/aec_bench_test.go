package aec

import (
	"encoding/base64"
	"testing"
)

func benchVector(b *testing.B, kindContains string) {
	vs := loadVectors(b)
	var stream, want []byte
	var cfg Config
	for _, v := range vs {
		if want == nil || containsStr(v.Name, kindContains) {
			want, _ = base64.StdEncoding.DecodeString(v.SamplesB64)
			stream, _ = base64.StdEncoding.DecodeString(v.StreamB64)
			cfg = Config{BitsPerSample: v.BitsPerSample, BlockSize: v.BlockSize, RSI: v.RSI, Flags: Flags(v.Flags)}
			if containsStr(v.Name, kindContains) {
				break
			}
		}
	}
	dst := make([]byte, len(want))
	b.SetBytes(int64(len(want)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Decode(dst, stream, cfg); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecodeRamp(b *testing.B)        { benchVector(b, "ramp") }
func BenchmarkDecodeZeros(b *testing.B)       { benchVector(b, "zeros") }
func BenchmarkDecodeHighEntropy(b *testing.B) { benchVector(b, "highentropy") }

func containsStr(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
