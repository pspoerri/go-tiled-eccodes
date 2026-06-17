//go:build libaec

package aec

// libaec_test.go uses the system libaec (via libaec_cgo.go) and is built only
// with `-tags libaec`. It (re)generates testdata/vectors.json. Install libaec
// first (macOS: `brew install libaec`).

import (
	"encoding/base64"
	"encoding/json"
	"math/rand"
	"os"
	"testing"
)

// aecEncode encodes raw sample bytes with libaec, returning the bitstream.
func aecEncode(t testing.TB, raw []byte, cfg Config) []byte {
	t.Helper()
	out, err := libaecEncode(raw, cfg)
	if err != nil {
		t.Fatalf("aecEncode: %v", err)
	}
	return out
}

// aecDecodeC decodes with libaec into a buffer of exactly outLen bytes.
func aecDecodeC(t testing.TB, stream []byte, outLen int, cfg Config) []byte {
	t.Helper()
	out, err := libaecDecode(stream, outLen, cfg)
	if err != nil {
		t.Fatalf("aecDecodeC: %v", err)
	}
	return out
}

// sweepVectors enumerates the parameter sweep + per-option payloads.
func sweepVectors(t testing.TB) []vector {
	rng := rand.New(rand.NewSource(1))
	bpsList := []int{1, 8, 9, 16, 17, 24, 25, 32}
	blockSizes := []int{8, 16, 32, 64}
	rsis := []int{1, 8, 128}
	var vs []vector
	add := func(name string, bps, bs, rsi int, flags Flags, nSamples int, gen func(i int) uint32) {
		bytesPer := bytesPerSampleFor(bps, flags)
		raw := make([]byte, nSamples*bytesPer)
		samples := make([]uint32, nSamples)
		msb := flags&DataMSB != 0
		for i := range samples {
			samples[i] = gen(i) & maskBits(bps)
			writeSample(raw, i, samples[i], bytesPer, msb)
		}
		cfg := Config{BitsPerSample: bps, BlockSize: bs, RSI: rsi, Flags: flags}
		stream := aecEncode(t, raw, cfg)
		// The authoritative expected output is what libaec's DECODER produces
		// (lossless => equals canonical input). Recording libaec's output rather
		// than the raw generator input makes the frozen vectors a true libaec
		// oracle even for non-canonical signed inputs.
		decoded := aecDecodeC(t, stream, len(raw), cfg)
		vs = append(vs, vector{
			Name: name, BitsPerSample: bps, BlockSize: bs, RSI: rsi, Flags: uint(flags),
			SamplesB64: base64.StdEncoding.EncodeToString(decoded),
			StreamB64:  base64.StdEncoding.EncodeToString(stream),
		})
	}

	// Broad random sweep across bps x blockSize x flag combos.
	for _, bps := range bpsList {
		flagsets := []Flags{
			DataPreprocess | DataMSB,
			DataPreprocess,
			DataMSB,
			0,
			DataPreprocess | DataMSB | DataSigned,
		}
		if bps > 16 && bps <= 24 {
			flagsets = append(flagsets, DataPreprocess|DataMSB|Data3Byte)
		}
		for _, fl := range flagsets {
			for _, bs := range blockSizes {
				for _, rsi := range rsis {
					name := vecName(bps, bs, rsi, fl, "rand")
					// Non-block-aligned count to exercise the trailing partial block.
					nSamples := min(bs*rsi*2, 4096) + 7
					add(name, bps, bs, rsi, fl, nSamples, func(i int) uint32 {
						return uint32(rng.Intn(1 << uint(min(bps, 16))))
					})
				}
			}
		}
	}
	// Per-option shaped payloads (preprocess+MSB, 16-bit, block 32, rsi 16).
	bps, bs, rsi, fl := 16, 32, 16, DataPreprocess|DataMSB
	add(vecName(bps, bs, rsi, fl, "zeros"), bps, bs, rsi, fl, bs*rsi*2+5, func(i int) uint32 { return 1000 })
	add(vecName(bps, bs, rsi, fl, "lowvar"), bps, bs, rsi, fl, bs*rsi*2+5, func(i int) uint32 { return uint32(1000 + i%2) })
	add(vecName(bps, bs, rsi, fl, "ramp"), bps, bs, rsi, fl, bs*rsi*2+5, func(i int) uint32 { return uint32(1000 + i) })
	add(vecName(bps, bs, rsi, fl, "highentropy"), bps, bs, rsi, fl, bs*rsi*2+5, func(i int) uint32 { return uint32(rng.Intn(1 << 16)) })
	return vs
}

// TestDifferentialLibaec decodes every swept input with BOTH the pure-Go
// decoder and libaec, asserting byte-for-byte equality. This is the
// authoritative cross-check; it only builds under -tags libaec.
func TestDifferentialLibaec(t *testing.T) {
	for _, v := range sweepVectors(t) {
		v := v
		t.Run(v.Name, func(t *testing.T) {
			want, _ := base64.StdEncoding.DecodeString(v.SamplesB64)
			stream, _ := base64.StdEncoding.DecodeString(v.StreamB64)
			cfg := Config{BitsPerSample: v.BitsPerSample, BlockSize: v.BlockSize, RSI: v.RSI, Flags: Flags(v.Flags)}

			goOut := make([]byte, len(want))
			n, err := Decode(goOut, stream, cfg)
			if err != nil {
				t.Fatalf("go decode: %v", err)
			}
			cOut := aecDecodeC(t, stream, len(want), cfg)
			if n != len(cOut) || string(goOut[:n]) != string(cOut) {
				// Find first differing byte for a useful message.
				for i := 0; i < len(cOut) && i < n; i++ {
					if goOut[i] != cOut[i] {
						t.Fatalf("byte %d: go=%d libaec=%d (cfg %+v)", i, goOut[i], cOut[i], cfg)
					}
				}
				t.Fatalf("length mismatch: go=%d libaec=%d", n, len(cOut))
			}
		})
	}
}

// TestGenerateVectors regenerates the frozen fixtures. Run explicitly:
//
//	go test -tags libaec ./aec/ -run TestGenerateVectors
func TestGenerateVectors(t *testing.T) {
	vs := sweepVectors(t)
	raw, err := json.MarshalIndent(vs, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll("testdata", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("testdata/vectors.json", raw, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %d vectors", len(vs))
}
