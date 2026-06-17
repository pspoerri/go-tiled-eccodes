# Pure-Go CCSDS (5.42) Decoder — libaec Port — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port libaec's CCSDS 121.0-B-3 adaptive entropy *decoder* to a public pure-Go `aec` package and make it the only CCSDS (5.42) path, dropping the libaec CGo dependency.

**Architecture:** A new `github.com/pspoerri/go-tiled-eccodes/aec` package faithfully reimplements libaec v1.1.7's `aec_buffer_decode`, producing byte-identical output. `decode/ccsds.go`'s `ccsdsDecode` becomes a thin adapter over `aec.Decode`; the CGo `ccsds_cgo.go`/`ccsds_nocgo.go` pair is deleted. JPEG2000 (5.40) keeps its CGo path and the `ErrCgoRequired` sentinel (moved to `decode/errors.go`). Correctness is anchored by frozen golden vectors generated from libaec's encoder, plus a retained `//go:build libaec` differential test that byte-compares against libaec.

**Tech Stack:** Go 1.26, `math/bits`; libaec (system, dev/test only, via CGo under `-tags libaec`) for vector generation and the differential test.

**Reference:** libaec v1.1.7 `src/decode.c` (`MathisRosenhauer/libaec`). All option/preprocessing arithmetic below is transcribed from that source. The design spec is `docs/superpowers/specs/2026-06-17-ccsds-libaec-pure-go-port-design.md`.

## Global Constraints

- **Module:** `github.com/pspoerri/go-tiled-eccodes`; Go directive `go 1.26.2` (do not lower).
- **Pure-Go default:** `go build ./...` and `go test ./...` must compile and pass with `CGO_ENABLED=0`. No new non-test runtime dependency. The only CGo in the tree after this work is JPEG2000 (`decode/jpeg2000_cgo.go`) and the `//go:build libaec` dev/test files.
- **Byte-exactness:** `aec.Decode` must reproduce libaec `aec_buffer_decode` output **byte-for-byte** for all valid configs. All multi-bit reads are **MSB-first**. A fundamental-sequence (FS) value is the count of leading **0** bits before the terminating **1**.
- **Endianness fixed by flag:** output bytes are big-endian iff `DataMSB` is set, else little-endian, for every storage width including 3-byte.
- **Allocation:** the steady-state decode path allocates nothing when the caller supplies an adequately sized `dst`; all scratch (RSI buffer, SE table) is allocated once at `Decode` entry.
- **Naming:** exported flag constants and `Config` field names exactly as in Task 1. Internal decoder type is `decoder` (lowercase).
- **libaec for `-tags libaec` steps:** macOS Homebrew `/opt/homebrew/{include,lib}`; linker `-laec`. These steps require libaec installed; the default build never does.
- **Commit** after every task with the message shown in its final step.

---

## File Structure

**New package `aec/`:**
- `aec/aec.go` — package doc, `Flags`, `Config`, `Decode` entry, exported errors, config→decoder setup + validation.
- `aec/bitreader.go` — MSB-first `bitReader`: `ask`/`get`/`drop`/`getBits`/`getFS`.
- `aec/setable.go` — second-extension inverse table (`buildSETable`, `seTableSize`).
- `aec/decoder.go` — `decoder` struct, `run` loop, block dispatch, the four code options, `flush`, `put`.
- `aec/aec_test.go` — golden-vector + hand-vector unit tests (pure Go).
- `aec/bitreader_test.go` — bit-reader unit tests.
- `aec/aec_bench_test.go` — pure-Go benchmarks.
- `aec/libaec_test.go` (`//go:build libaec`) — fixture generator, differential test, libaec baseline benchmark.
- `aec/testdata/vectors.json` — frozen golden vectors (generated, committed).

**Modified `decode/`:**
- `decode/ccsds.go` — `ccsdsDecode` removed from here (it lived in the build-tagged files); doc comment for `ErrCgoRequired` deleted (var moves out). Body of `CCSDS` unchanged.
- `decode/ccsds_decode.go` (new) — unconditional `ccsdsDecode` adapter over `aec.Decode`.
- `decode/ccsds_cgo.go`, `decode/ccsds_nocgo.go` — **deleted**.
- `decode/errors.go` — gains `ErrCgoRequired`.

**Modified repo-level tests/docs:**
- `ccsds_test.go` (new, unconditional) — CCSDS end-to-end tests moved out of the cgo file.
- `ccsds_jpeg2000_test.go` — CCSDS tests removed (JPEG2000 tests stay, still `//go:build cgo`).
- `ccsds_jpeg2000_nocgo_test.go` — CCSDS assertion removed (JPEG2000-only).
- `bench_test.go` — add `BenchmarkDecodeICONCCSDS`.
- `README.md` — CCSDS moves to pure-Go in prose + Section 5 table.

---

## Task 1: Scaffold `aec` package — public API, errors, config validation

**Files:**
- Create: `aec/aec.go`
- Test: `aec/aec_test.go`

**Interfaces:**
- Produces: `type Flags uint`; constants `DataSigned, Data3Byte, DataMSB, DataPreprocess, RestrictedCodes, PadRSI Flags`; `type Config struct { BitsPerSample, BlockSize, RSI int; Flags Flags }`; `func Decode(dst, src []byte, cfg Config) (int, error)`; errors `ErrConfig, ErrData, ErrShortInput, ErrShortOutput`; internal `func newDecoder(dst, src []byte, cfg Config) (*decoder, error)` returning a partially-initialised `decoder` (fields filled here; `run` added in Task 4). For this task `Decode` validates config and returns `ErrConfig` for invalid configs, otherwise `(0, nil)` as a temporary stub (replaced in Task 4).

- [ ] **Step 1: Write the failing test**

Create `aec/aec_test.go`:

```go
package aec

import (
	"errors"
	"testing"
)

func TestConfigValidation(t *testing.T) {
	good := Config{BitsPerSample: 16, BlockSize: 32, RSI: 128, Flags: DataPreprocess | DataMSB}
	cases := []struct {
		name string
		cfg  Config
		want error
	}{
		{"ok", good, nil},
		{"bps0", Config{BitsPerSample: 0, BlockSize: 32, RSI: 128}, ErrConfig},
		{"bps33", Config{BitsPerSample: 33, BlockSize: 32, RSI: 128}, ErrConfig},
		{"rsi0", Config{BitsPerSample: 16, BlockSize: 32, RSI: 0}, ErrConfig},
		{"rsi4097", Config{BitsPerSample: 16, BlockSize: 32, RSI: 4097}, ErrConfig},
		{"blk0", Config{BitsPerSample: 16, BlockSize: 0, RSI: 128}, ErrConfig},
		{"blkOdd", Config{BitsPerSample: 16, BlockSize: 31, RSI: 128}, ErrConfig},
		{"blk257", Config{BitsPerSample: 16, BlockSize: 258, RSI: 128}, ErrConfig},
		{"restrictedBps6", Config{BitsPerSample: 6, BlockSize: 32, RSI: 128, Flags: RestrictedCodes}, ErrConfig},
		{"restrictedBps4ok", Config{BitsPerSample: 4, BlockSize: 32, RSI: 128, Flags: RestrictedCodes}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := newDecoder(make([]byte, 1<<16), []byte{0, 0, 0, 0}, c.cfg)
			if !errors.Is(err, c.want) && err != c.want {
				t.Fatalf("newDecoder err = %v, want %v", err, c.want)
			}
		})
	}
}

func TestDerivedParams(t *testing.T) {
	cases := []struct {
		bps, idLen, bps2 int
		flags            Flags
	}{
		{8, 3, 1, 0}, {1, 3, 1, 0}, {9, 4, 2, 0}, {16, 4, 2, 0},
		{17, 5, 4, 0}, {24, 5, 4, 0}, {24, 5, 3, Data3Byte}, {32, 5, 4, 0},
		{2, 1, 1, RestrictedCodes}, {4, 2, 1, RestrictedCodes},
	}
	for _, c := range cases {
		d, err := newDecoder(make([]byte, 1<<16), []byte{0}, Config{BitsPerSample: c.bps, BlockSize: 16, RSI: 8, Flags: c.flags})
		if err != nil {
			t.Fatalf("bps=%d: %v", c.bps, err)
		}
		if d.idLen != c.idLen || d.bytesPerSample != c.bps2 {
			t.Fatalf("bps=%d flags=%d: idLen=%d (want %d) bytesPerSample=%d (want %d)",
				c.bps, c.flags, d.idLen, c.idLen, d.bytesPerSample, c.bps2)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./aec/`
Expected: FAIL — `undefined: newDecoder` / `undefined: Config` (package does not compile).

- [ ] **Step 3: Write the implementation**

Create `aec/aec.go`:

```go
// Package aec is a pure-Go decoder for CCSDS 121.0-B-3 adaptive entropy
// coding — the algorithm implemented by libaec and used by GRIB2 Data
// Representation Template 5.42. It is a faithful port of libaec v1.1.7's
// aec_buffer_decode and produces byte-identical output.
//
// Only decoding of a complete buffer is supported (the equivalent of
// libaec's aec_buffer_decode); there is no encoder and no streaming API.
package aec

import "errors"

// Flags mirrors libaec's sample-data description flags. The bit values are
// identical to <libaec.h> so a GRIB2 "CCSDS compression options" byte maps
// straight onto a Flags value.
type Flags uint

const (
	DataSigned      Flags = 1 << iota // samples are signed (two's complement)
	Data3Byte                         // 24-bit samples stored in 3 bytes
	DataMSB                           // output bytes most-significant-first (big-endian)
	DataPreprocess                    // preprocessor (predictor) was applied
	RestrictedCodes                   // restricted set of code options
	PadRSI                            // each RSI padded to a byte boundary
)

// Config describes the bitstream being decoded. The values come from the
// GRIB2 template (or the encoder that produced the stream).
type Config struct {
	BitsPerSample int // 1..32
	BlockSize     int // 8, 16, 32, 64 (must be even, 2..256)
	RSI           int // reference sample interval, in blocks (1..4096)
	Flags         Flags
}

// Exported errors. They wrap the libaec failure modes.
var (
	ErrConfig      = errors.New("aec: invalid configuration")
	ErrData        = errors.New("aec: malformed bitstream")
	ErrShortInput  = errors.New("aec: input ended before all samples were decoded")
	ErrShortOutput = errors.New("aec: dst too small for decoded samples")
)

// Decode decodes the AEC bitstream src into dst, writing BitsPerSample-wide
// samples in the byte layout libaec produces: storage width 1/2/3/4 bytes
// (per BitsPerSample and Data3Byte), big-endian iff DataMSB else
// little-endian. It returns the number of bytes written. dst must be large
// enough for all samples it can decode from src; Decode never grows dst.
func Decode(dst, src []byte, cfg Config) (int, error) {
	d, err := newDecoder(dst, src, cfg)
	if err != nil {
		return 0, err
	}
	if err := d.run(); err != nil {
		return d.outPos, err
	}
	return d.outPos, nil
}

// newDecoder validates cfg and builds a decoder with all derived parameters
// set. It mirrors libaec's aec_decode_init.
func newDecoder(dst, src []byte, cfg Config) (*decoder, error) {
	bps := cfg.BitsPerSample
	if bps < 1 || bps > 32 ||
		cfg.RSI < 1 || cfg.RSI > 4096 ||
		cfg.BlockSize < 2 || cfg.BlockSize > 256 || cfg.BlockSize&1 != 0 {
		return nil, ErrConfig
	}

	d := &decoder{
		cfg:       cfg,
		blockSize: cfg.BlockSize,
		rsi:       cfg.RSI,
		rsiSize:   cfg.RSI * cfg.BlockSize,
		pp:        cfg.Flags&DataPreprocess != 0,
		signed:    cfg.Flags&DataSigned != 0,
		msb:       cfg.Flags&DataMSB != 0,
	}

	switch {
	case bps > 16:
		d.idLen = 5
		if bps <= 24 && cfg.Flags&Data3Byte != 0 {
			d.bytesPerSample = 3
		} else {
			d.bytesPerSample = 4
		}
	case bps > 8:
		d.idLen = 4
		d.bytesPerSample = 2
	default: // bps 1..8
		if cfg.Flags&RestrictedCodes != 0 {
			if bps > 4 {
				return nil, ErrConfig // libaec rejects RESTRICTED with bps 5..8
			}
			if bps <= 2 {
				d.idLen = 1
			} else {
				d.idLen = 2
			}
		} else {
			d.idLen = 3
		}
		d.bytesPerSample = 1
	}
	d.idMax = uint32(1)<<uint(d.idLen) - 1

	// Sample range. unsignedMax = 2^bps - 1 (all-ones in bps bits).
	unsignedMax := ^uint32(0) >> uint(32-bps)
	if d.signed {
		d.xmax = unsignedMax >> 1 // 2^(bps-1) - 1
		d.xmin = ^d.xmax
	} else {
		d.xmin = 0
		d.xmax = unsignedMax
	}

	d.seTable = buildSETable()
	d.rsiBuf = make([]uint32, d.rsiSize)
	d.br = bitReader{src: src}
	d.dst = dst
	d.needed = len(dst) / d.bytesPerSample
	return d, nil
}
```

Add a minimal `decoder` definition so the package compiles. It will be fleshed out in Task 4 — define it now in `aec/decoder.go`:

```go
package aec

// decoder holds all per-Decode state. Built by newDecoder; driven by run.
type decoder struct {
	cfg            Config
	idLen          int
	idMax          uint32
	bytesPerSample int
	blockSize      int
	rsi            int // blocks per RSI
	rsiSize        int // samples per RSI
	xmin, xmax     uint32
	pp, signed     bool
	msb            bool

	seTable []int
	rsiBuf  []uint32
	rsip    int // samples buffered in the current RSI

	br      bitReader
	dst     []byte
	outPos  int    // bytes written to dst
	needed  int    // total samples to emit
	emitted int    // samples emitted
	lastOut uint32 // predictor carry
}

// run is implemented in Task 4.
func (d *decoder) run() error { return nil }
```

And stub the bit reader / SE table so the package builds — create `aec/bitreader.go`:

```go
package aec

// bitReader is replaced with a real implementation in Task 2.
type bitReader struct {
	src []byte
	pos int
	acc uint64
	cnt int
}
```

and `aec/setable.go`:

```go
package aec

// buildSETable is replaced with a real implementation in Task 3.
func buildSETable() []int { return make([]int, 182) }
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./aec/ -run 'TestConfigValidation|TestDerivedParams' -v`
Expected: PASS (both tests).

- [ ] **Step 5: Commit**

```bash
git add aec/aec.go aec/decoder.go aec/bitreader.go aec/setable.go aec/aec_test.go
git commit -m "aec: scaffold package, public API, config validation"
```

---

## Task 2: MSB-first bit reader

**Files:**
- Modify: `aec/bitreader.go`
- Test: `aec/bitreader_test.go`

**Interfaces:**
- Consumes: `bitReader struct { src []byte; pos int; acc uint64; cnt int }` from Task 1.
- Produces: methods `func (b *bitReader) getBits(n int) (uint32, bool)` (reads n MSB-first bits, n in 0..32; false if input exhausted), `func (b *bitReader) getFS() (uint32, bool)` (unary: count leading zeros before a 1; consumes the terminating 1; false if exhausted). These mirror libaec `bits_ask`+`bits_get`+`bits_drop` and `fs_ask`+`fs_drop`.

- [ ] **Step 1: Write the failing test**

Create `aec/bitreader_test.go`:

```go
package aec

import "testing"

func TestGetBitsMSB(t *testing.T) {
	// 0xB4 = 1011_0100, 0x2D = 0010_1101.
	b := bitReader{src: []byte{0xB4, 0x2D}}
	// Read 3 bits: 101 = 5.
	if v, ok := b.getBits(3); !ok || v != 0b101 {
		t.Fatalf("getBits(3) = %d,%v want 5,true", v, ok)
	}
	// Next 5 bits: 10100 = 20.
	if v, ok := b.getBits(5); !ok || v != 0b10100 {
		t.Fatalf("getBits(5) = %d,%v want 20,true", v, ok)
	}
	// Next 8 bits span into byte 2: 00101101 = 0x2D.
	if v, ok := b.getBits(8); !ok || v != 0x2D {
		t.Fatalf("getBits(8) = %d,%v want 45,true", v, ok)
	}
	if _, ok := b.getBits(1); ok {
		t.Fatalf("getBits past end should fail")
	}
}

func TestGetBitsZero(t *testing.T) {
	b := bitReader{src: []byte{0xFF}}
	if v, ok := b.getBits(0); !ok || v != 0 {
		t.Fatalf("getBits(0) = %d,%v want 0,true", v, ok)
	}
}

func TestGetFS(t *testing.T) {
	// 0001_1000: FS values: 3 (000 then 1), then 0 (1), then bits 000 left.
	b := bitReader{src: []byte{0b0001_1000}}
	if v, ok := b.getFS(); !ok || v != 3 {
		t.Fatalf("getFS#1 = %d,%v want 3,true", v, ok)
	}
	if v, ok := b.getFS(); !ok || v != 0 {
		t.Fatalf("getFS#2 = %d,%v want 0,true", v, ok)
	}
	// Remaining 000 with no terminating 1 -> exhausted.
	if _, ok := b.getFS(); ok {
		t.Fatalf("getFS#3 should fail (no terminating 1)")
	}
}

func TestGetFSAcrossBytes(t *testing.T) {
	// 12 leading zeros then 1: 0x00, 0x08 = 0000_0000 0000_1000.
	b := bitReader{src: []byte{0x00, 0x08}}
	if v, ok := b.getFS(); !ok || v != 12 {
		t.Fatalf("getFS = %d,%v want 12,true", v, ok)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./aec/ -run TestGet -v`
Expected: FAIL — `b.getBits undefined`, `b.getFS undefined`.

- [ ] **Step 3: Write the implementation**

Replace `aec/bitreader.go` entirely:

```go
package aec

import "math"

// bitReader pulls bits MSB-first from src. Valid (unconsumed) bits occupy the
// low `cnt` bits of acc, the oldest unconsumed bit being the most significant
// of those. This mirrors libaec's bits_ask/bits_get/bits_drop accumulator.
type bitReader struct {
	src []byte
	pos int    // index of next byte to load
	acc uint64 // bit accumulator; meaningful bits are acc[cnt-1 .. 0]
	cnt int    // number of valid bits in acc (stays < 56)
}

// ask ensures at least n (<=32) bits are buffered, loading bytes big-endian.
// Returns false if src is exhausted first.
func (b *bitReader) ask(n int) bool {
	for b.cnt < n {
		if b.pos >= len(b.src) {
			return false
		}
		b.acc = b.acc<<8 | uint64(b.src[b.pos])
		b.pos++
		b.cnt += 8
	}
	return true
}

// getBits reads the next n MSB-first bits (n in 0..32). The first bit read is
// the most significant of the result.
func (b *bitReader) getBits(n int) (uint32, bool) {
	if n == 0 {
		return 0, true
	}
	if !b.ask(n) {
		return 0, false
	}
	v := uint32((b.acc >> uint(b.cnt-n)) & (math.MaxUint64 >> uint(64-n)))
	b.cnt -= n
	return v, true
}

// getFS reads a fundamental-sequence value: the number of consecutive 0 bits
// before the next 1 bit. The terminating 1 is consumed. Mirrors fs_ask+fs_drop.
func (b *bitReader) getFS() (uint32, bool) {
	var fs uint32
	if !b.ask(1) {
		return 0, false
	}
	for b.acc&(uint64(1)<<uint(b.cnt-1)) == 0 {
		if b.cnt == 1 {
			if b.pos >= len(b.src) {
				return 0, false
			}
			b.acc = b.acc<<8 | uint64(b.src[b.pos])
			b.pos++
			b.cnt += 8
		}
		fs++
		b.cnt--
	}
	b.cnt-- // drop the terminating 1
	return fs, true
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./aec/ -run TestGet -v`
Expected: PASS (all four tests).

- [ ] **Step 5: Commit**

```bash
git add aec/bitreader.go aec/bitreader_test.go
git commit -m "aec: MSB-first bit reader (getBits, getFS)"
```

---

## Task 3: Second-extension inverse table

**Files:**
- Modify: `aec/setable.go`
- Test: `aec/aec_test.go` (append)

**Interfaces:**
- Produces: `const seTableSize = 90`; `func buildSETable() []int` returning a length-182 slice where, for a decoded FS value `m` (0..90), `table[2m]` is the pair sum `d0+d1` and `table[2m+1]` is the row base `i*(i+1)/2`. Mirrors libaec `create_se_table`.

- [ ] **Step 1: Write the failing test**

Append to `aec/aec_test.go`:

```go
func TestSETable(t *testing.T) {
	tab := buildSETable()
	if len(tab) != 2*(seTableSize+1) {
		t.Fatalf("len = %d, want %d", len(tab), 2*(seTableSize+1))
	}
	// For every m, the decoded pair must invert the forward triangular map
	// gamma = (d0+d1)(d0+d1+1)/2 + d1.
	for m := 0; m <= seTableSize; m++ {
		total := tab[2*m]   // d0 + d1
		ms := tab[2*m+1]    // row base = total*(total+1)/2
		d1 := m - ms
		d0 := total - d1
		if d0 < 0 || d1 < 0 || d1 > total {
			t.Fatalf("m=%d: bad pair d0=%d d1=%d total=%d", m, d0, d1, total)
		}
		gamma := (d0+d1)*(d0+d1+1)/2 + d1
		if gamma != m {
			t.Fatalf("m=%d: forward map gives %d (d0=%d d1=%d)", m, gamma, d0, d1)
		}
	}
	// Spot-check the first rows: m=0 ->(0,0); m=1->total1 base1; m=2->total2 base3.
	if tab[0] != 0 || tab[1] != 0 {
		t.Fatalf("m=0 entry = (%d,%d) want (0,0)", tab[0], tab[1])
	}
	if tab[2*1] != 1 || tab[2*1+1] != 1 {
		t.Fatalf("m=1 entry = (%d,%d) want (1,1)", tab[2], tab[3])
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./aec/ -run TestSETable -v`
Expected: FAIL — current stub returns all zeros, so the forward-map check fails at m=1.

- [ ] **Step 3: Write the implementation**

Replace `aec/setable.go` entirely:

```go
package aec

// seTableSize is the largest FS value the second-extension table covers
// (libaec's SE_TABLE_SIZE). FS values above this are a data error.
const seTableSize = 90

// buildSETable precomputes the inverse of the second-extension triangular
// map. For a decoded FS value m, table[2m] = d0+d1 (the pair sum) and
// table[2m+1] = the row base (sum)*(sum+1)/2. Mirrors libaec create_se_table.
func buildSETable() []int {
	table := make([]int, 2*(seTableSize+1))
	k := 0
	for i := 0; i < 13; i++ {
		ms := k
		for j := 0; j <= i; j++ {
			table[2*k] = i
			table[2*k+1] = ms
			k++
		}
	}
	return table
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./aec/ -run TestSETable -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add aec/setable.go aec/aec_test.go
git commit -m "aec: second-extension inverse table"
```

---

## Task 4: Decoder core — loop, flush, output serialization, preprocessing (uncompressed + no-pp first)

This task builds the decode loop, output `put`, and the `flush` predictor reversal, and implements the **uncompressed** option end-to-end. The other three options are added in Tasks 5–7. Until then they return `ErrData` so the package compiles and partial fixtures pass.

**Files:**
- Modify: `aec/decoder.go`
- Test: `aec/aec_test.go` (append hand-built vectors)

**Interfaces:**
- Consumes: `bitReader.getBits/getFS` (Task 2); `decoder` fields + `seTable` (Tasks 1, 3).
- Produces: `func (d *decoder) run() error`; `func (d *decoder) decodeBlock() error`; `func (d *decoder) uncomp() error`; `func (d *decoder) flush(buf []uint32)`; `func (d *decoder) put(v uint32)`. Stubs `func (d *decoder) split(k, ref int) error`, `func (d *decoder) lowEntropy(ref int) error` returning `ErrData`.

- [ ] **Step 1: Write the failing test**

Append to `aec/aec_test.go`. These vectors are computed by hand from the algorithm (no libaec needed):

```go
// helper: build a uint32 sample buffer into MSB/LSB bytes the way Decode emits.
func packSamples(samples []uint32, bytesPer int, msb bool) []byte {
	out := make([]byte, len(samples)*bytesPer)
	for i, v := range samples {
		o := i * bytesPer
		switch bytesPer {
		case 1:
			out[o] = byte(v)
		case 2:
			if msb {
				out[o], out[o+1] = byte(v>>8), byte(v)
			} else {
				out[o], out[o+1] = byte(v), byte(v>>8)
			}
		case 4:
			if msb {
				out[o], out[o+1], out[o+2], out[o+3] = byte(v>>24), byte(v>>16), byte(v>>8), byte(v)
			} else {
				out[o], out[o+1], out[o+2], out[o+3] = byte(v), byte(v>>8), byte(v>>16), byte(v>>24)
			}
		}
	}
	return out
}

// TestUncompNoPP: one block of 8 uncompressed 8-bit samples, no preprocessing.
// id_len=3, id_max=7 -> uncompressed id is 0b111. Then 8 raw 8-bit samples.
func TestUncompNoPP(t *testing.T) {
	cfg := Config{BitsPerSample: 8, BlockSize: 8, RSI: 2, Flags: 0}
	samples := []uint32{10, 20, 30, 40, 250, 1, 2, 3}
	// Bitstream: id (111) then each sample as 8 bits.
	var bw bitWriter
	bw.put(0b111, 3)
	for _, s := range samples {
		bw.put(s, 8)
	}
	dst := make([]byte, len(samples)) // exactly 8 bytes -> needed=8 samples (one block)
	n, err := Decode(dst, bw.bytes(), cfg)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	want := packSamples(samples, 1, false)
	if n != len(want) || string(dst[:n]) != string(want) {
		t.Fatalf("got %v (n=%d), want %v", dst[:n], n, want)
	}
}

// TestUncompPPUnsigned: preprocessing on, unsigned 16-bit. First sample is the
// raw reference; subsequent stored values are mapped residuals reversed by the
// predictor. We pick residuals that stay in range so the zig-zag branch applies.
func TestUncompPPUnsigned(t *testing.T) {
	cfg := Config{BitsPerSample: 16, BlockSize: 8, RSI: 2, Flags: DataPreprocess | DataMSB}
	// reference=1000. residuals d: 2->+1, 4->+2, 1->-1, 0->0, 6->+3, 3->-2, 8->+4
	// (even d -> +d/2, odd d -> -(d+1)/2), accumulated onto the predictor.
	stored := []uint32{1000, 2, 4, 1, 0, 6, 3, 8}
	wantSamples := []uint32{1000, 1001, 1003, 1002, 1002, 1005, 1003, 1007}
	var bw bitWriter
	bw.put(0b1111, 4) // id_len=4, id_max=15
	for _, s := range stored {
		bw.put(s, 16)
	}
	dst := make([]byte, len(stored)*2)
	n, err := Decode(dst, bw.bytes(), cfg)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	want := packSamples(wantSamples, 2, true)
	if n != len(want) || string(dst[:n]) != string(want) {
		t.Fatalf("got %v, want %v", dst[:n], want)
	}
}
```

Also append a tiny MSB-first bit writer used only by tests (place in `aec/aec_test.go`):

```go
// bitWriter writes bits MSB-first, the same order bitReader consumes them.
type bitWriter struct {
	buf []byte
	acc uint64
	cnt int
}

func (w *bitWriter) put(v uint32, n int) {
	w.acc = w.acc<<uint(n) | uint64(v&(1<<uint(n)-1))
	w.cnt += n
	for w.cnt >= 8 {
		w.cnt -= 8
		w.buf = append(w.buf, byte(w.acc>>uint(w.cnt)))
	}
}

func (w *bitWriter) bytes() []byte {
	out := w.buf
	if w.cnt > 0 {
		out = append(out, byte(w.acc<<uint(8-w.cnt))) // pad final byte with zeros
	}
	return out
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./aec/ -run 'TestUncomp' -v`
Expected: FAIL — `run` is a stub returning nil so nothing is written (`n=0`).

- [ ] **Step 3: Write the implementation**

Replace the `run` stub in `aec/decoder.go` and add the methods. Replace `func (d *decoder) run() error { return nil }` with:

```go
// run drives the block loop until enough samples are decoded to fill dst,
// then flushes the trailing partial RSI. Mirrors aec_decode's main loop for
// the whole-buffer (AEC_FLUSH) case.
func (d *decoder) run() error {
	for d.emitted+d.rsip < d.needed {
		if err := d.decodeBlock(); err != nil {
			return err
		}
		if d.rsip >= d.rsiSize {
			d.flush(d.rsiBuf[:d.rsiSize])
			d.rsip = 0
		}
	}
	if d.rsip > 0 {
		d.flush(d.rsiBuf[:d.rsip])
		d.rsip = 0
	}
	return nil
}

// decodeBlock reads one block's option id and dispatches. The first block of
// each RSI (when preprocessing) carries a reference sample.
func (d *decoder) decodeBlock() error {
	ref := 0
	if d.pp && d.rsip == 0 {
		ref = 1
	}
	id, ok := d.br.getBits(d.idLen)
	if !ok {
		return ErrShortInput
	}
	switch {
	case id == 0:
		return d.lowEntropy(ref)
	case id == d.idMax:
		return d.uncomp()
	default:
		return d.split(int(id)-1, ref)
	}
}

// uncomp reads block_size raw bits_per_sample literals. (libaec m_uncomp reads
// the full block regardless of ref; slot 0 of the RSI is implicitly the
// reference and is treated as such at flush time.)
func (d *decoder) uncomp() error {
	for i := 0; i < d.blockSize; i++ {
		v, ok := d.br.getBits(d.cfg.BitsPerSample)
		if !ok {
			return ErrShortInput
		}
		d.rsiBuf[d.rsip] = v
		d.rsip++
	}
	return nil
}

// split / lowEntropy are implemented in later tasks.
func (d *decoder) split(k, ref int) error { return ErrData }
func (d *decoder) lowEntropy(ref int) error { return ErrData }

// flush reverses preprocessing (if enabled) over a full RSI buffer and
// serializes the samples to dst. With whole-buffer decode, flush always starts
// at buf[0], so the reference is buf[0] and the predictor resets each RSI.
// Mirrors libaec's FLUSH macro.
func (d *decoder) flush(buf []uint32) {
	if !d.pp {
		for _, v := range buf {
			if d.emitted >= d.needed {
				return
			}
			d.put(v)
		}
		return
	}

	last := buf[0]
	if d.signed {
		m := uint32(1) << uint(d.cfg.BitsPerSample-1)
		last = (last ^ m) - m // sign-extend the reference
	}
	d.put(last)
	data := last
	xmax := d.xmax
	if d.xmin == 0 {
		med := xmax/2 + 1
		for _, dd := range buf[1:] {
			if d.emitted >= d.needed {
				break
			}
			halfD := dd>>1 + dd&1
			var mask uint32
			if data&med != 0 {
				mask = xmax
			}
			if halfD <= mask^data {
				data += dd>>1 ^ ^(dd&1 - 1)
			} else {
				data = mask ^ dd
			}
			d.put(data)
		}
	} else {
		for _, dd := range buf[1:] {
			if d.emitted >= d.needed {
				break
			}
			halfD := dd>>1 + dd&1
			if int32(data) < 0 {
				if halfD <= xmax+data+1 {
					data += dd>>1 ^ ^(dd&1 - 1)
				} else {
					data = dd - xmax - 1
				}
			} else {
				if halfD <= xmax-data {
					data += dd>>1 ^ ^(dd&1 - 1)
				} else {
					data = xmax - dd
				}
			}
			d.put(data)
		}
	}
	d.lastOut = data
}

// put serializes one sample to dst at the configured width and endianness,
// capped at the number of samples that fit. Mirrors put_msb_*/put_lsb_*.
func (d *decoder) put(v uint32) {
	if d.emitted >= d.needed {
		return
	}
	o := d.outPos
	switch d.bytesPerSample {
	case 1:
		d.dst[o] = byte(v)
	case 2:
		if d.msb {
			d.dst[o], d.dst[o+1] = byte(v>>8), byte(v)
		} else {
			d.dst[o], d.dst[o+1] = byte(v), byte(v>>8)
		}
	case 3:
		if d.msb {
			d.dst[o], d.dst[o+1], d.dst[o+2] = byte(v>>16), byte(v>>8), byte(v)
		} else {
			d.dst[o], d.dst[o+1], d.dst[o+2] = byte(v), byte(v>>8), byte(v>>16)
		}
	case 4:
		if d.msb {
			d.dst[o], d.dst[o+1], d.dst[o+2], d.dst[o+3] = byte(v>>24), byte(v>>16), byte(v>>8), byte(v)
		} else {
			d.dst[o], d.dst[o+1], d.dst[o+2], d.dst[o+3] = byte(v), byte(v>>8), byte(v>>16), byte(v>>24)
		}
	}
	d.outPos += d.bytesPerSample
	d.emitted++
}
```

> **Note on the zig-zag term:** `dd>>1 ^ ^(dd&1 - 1)` is Go for libaec's `(d>>1) ^ (~((d&1)-1))`. Go operator precedence makes this `(dd>>1) ^ (^((dd&1) - 1))`; the inner `^` is unary NOT on `uint32`, relying on wrap (`0-1 == 0xFFFFFFFF`). Even `dd` → `+dd/2`; odd `dd` → `-(dd+1)/2` in two's complement. The `put`/`run` cap on `needed` is what lets a non-block-aligned sample count (the GRIB norm — the encoder zero-pads the final block) decode correctly: extra padded samples are decoded into the RSI buffer but never written.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./aec/ -run 'TestUncomp' -v`
Expected: PASS (both `TestUncompNoPP` and `TestUncompPPUnsigned`).

- [ ] **Step 5: Commit**

```bash
git add aec/decoder.go aec/aec_test.go
git commit -m "aec: decode loop, flush/preprocessing, put, uncompressed option"
```

---

## Task 5: k-split (sample-splitting) option

**Files:**
- Modify: `aec/decoder.go`
- Test: `aec/aec_test.go` (append)

**Interfaces:**
- Produces: real `func (d *decoder) split(k, ref int) error` (replaces the Task 4 stub). Reads the reference sample when `ref==1`, then `block_size-ref` FS high parts, then (if `k>0`) `block_size-ref` k-bit remainders; `sample = (fs<<k) + remainder`.

- [ ] **Step 1: Write the failing test**

Append to `aec/aec_test.go`:

```go
// TestSplitNoPP: id selects k-split. id_len=3, so id in 1..6 -> k=id-1.
// Use id=3 -> k=2. block_size=4, no preprocessing, 8-bit. For each sample:
// high part fs (unary, fs zeros then 1) and 2-bit remainder; sample=(fs<<2)|rem.
func TestSplitNoPP(t *testing.T) {
	cfg := Config{BitsPerSample: 8, BlockSize: 4, RSI: 4, Flags: 0}
	fs := []uint32{0, 1, 2, 0}    // high parts
	rem := []uint32{1, 2, 3, 0}   // 2-bit low parts
	want := []uint32{0<<2 | 1, 1<<2 | 2, 2<<2 | 3, 0<<2 | 0} // 1,6,11,0
	var bw bitWriter
	bw.put(3, 3) // id=3 -> k=2
	for _, f := range fs { // FS: f zeros then a 1
		bw.put(1, int(f)+1)
	}
	for _, r := range rem {
		bw.put(r, 2)
	}
	dst := make([]byte, 4)
	n, err := Decode(dst, bw.bytes(), cfg)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	got := dst[:n]
	for i := range want {
		if uint32(got[i]) != want[i] {
			t.Fatalf("sample %d = %d, want %d", i, got[i], want[i])
		}
	}
}

// TestSplitK0NoPP: id=1 -> k=0 (pure fundamental sequence, no remainder bits).
func TestSplitK0NoPP(t *testing.T) {
	cfg := Config{BitsPerSample: 8, BlockSize: 4, RSI: 4, Flags: 0}
	fs := []uint32{5, 0, 3, 7}
	var bw bitWriter
	bw.put(1, 3) // id=1 -> k=0
	for _, f := range fs {
		bw.put(1, int(f)+1)
	}
	dst := make([]byte, 4)
	n, err := Decode(dst, bw.bytes(), cfg)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	for i, f := range fs {
		if uint32(dst[:n][i]) != f {
			t.Fatalf("sample %d = %d, want %d", i, dst[i], f)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./aec/ -run 'TestSplit' -v`
Expected: FAIL — `split` returns `ErrData`.

- [ ] **Step 3: Write the implementation**

In `aec/decoder.go` replace `func (d *decoder) split(k, ref int) error { return ErrData }` with:

```go
// split decodes a sample-splitting block with split parameter k (= id-1).
// libaec stores all encoded_block_size FS high parts first, then all k-bit
// remainders. When ref==1 the first slot is a raw reference sample.
func (d *decoder) split(k, ref int) error {
	if ref == 1 {
		v, ok := d.br.getBits(d.cfg.BitsPerSample)
		if !ok {
			return ErrShortInput
		}
		d.rsiBuf[d.rsip] = v
		d.rsip++
	}
	ebs := d.blockSize - ref
	base := d.rsip
	for i := 0; i < ebs; i++ {
		fs, ok := d.br.getFS()
		if !ok {
			return ErrShortInput
		}
		d.rsiBuf[base+i] = fs << uint(k)
	}
	if k > 0 {
		for i := 0; i < ebs; i++ {
			rem, ok := d.br.getBits(k)
			if !ok {
				return ErrShortInput
			}
			d.rsiBuf[base+i] += rem
		}
	}
	d.rsip = base + ebs
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./aec/ -run 'TestSplit' -v`
Expected: PASS (both).

- [ ] **Step 5: Commit**

```bash
git add aec/decoder.go aec/aec_test.go
git commit -m "aec: k-split (sample-splitting) option"
```

---

## Task 6: Zero-block (with ROS) and second-extension options

These two share the `m_low_entropy` dispatch (id==0 then a 1-bit sub-id: 1→second extension, 0→zero block), so they land together.

**Files:**
- Modify: `aec/decoder.go`
- Test: `aec/aec_test.go` (append)

**Interfaces:**
- Produces: real `func (d *decoder) lowEntropy(ref int) error` (replaces Task 4 stub); `func (d *decoder) zeroBlock(ref int) error`; `func (d *decoder) secondExtension(ref int) error`.

- [ ] **Step 1: Write the failing test**

Append to `aec/aec_test.go`:

```go
// TestZeroBlockNoPP: id=0 then sub-id=0 -> zero block. FS value f -> zero_blocks
// = f+1 (for f+1 < 5). One zero block = block_size zero samples. Then a second,
// uncompressed block so the RSI has some non-zero data too.
func TestZeroBlockNoPP(t *testing.T) {
	cfg := Config{BitsPerSample: 8, BlockSize: 4, RSI: 4, Flags: 0}
	var bw bitWriter
	// Block 1: zero block, f=1 -> zero_blocks=2 -> 8 zero samples (2 blocks).
	bw.put(0, 3)      // id=0 (low entropy)
	bw.put(0, 1)      // sub-id 0 -> zero block
	bw.put(1, 2)      // FS: f=1 (one 0 then 1)
	// Block 3 (index 2): uncompressed 4 samples.
	bw.put(7, 3)      // id_max -> uncompressed
	for _, s := range []uint32{9, 8, 7, 6} {
		bw.put(s, 8)
	}
	dst := make([]byte, 12) // 8 zeros + 4 samples
	n, err := Decode(dst, bw.bytes(), cfg)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	want := []byte{0, 0, 0, 0, 0, 0, 0, 0, 9, 8, 7, 6}
	if n != len(want) || string(dst[:n]) != string(want) {
		t.Fatalf("got %v, want %v", dst[:n], want)
	}
}

// TestSecondExtensionNoPP: id=0 then sub-id=1 -> second extension. Each FS value
// m maps to a pair (d0,d1) via the SE table; one gamma per pair, block_size/2
// gammas. With no preprocessing, the pair values are the output samples.
func TestSecondExtensionNoPP(t *testing.T) {
	cfg := Config{BitsPerSample: 8, BlockSize: 4, RSI: 4, Flags: 0}
	tab := buildSETable()
	ms := []uint32{4, 2} // two gammas -> two pairs -> 4 samples
	var want []uint32
	for _, m := range ms {
		d1 := int(m) - tab[2*m+1]
		d0 := tab[2*m] - d1
		want = append(want, uint32(d0), uint32(d1))
	}
	var bw bitWriter
	bw.put(0, 3) // id=0
	bw.put(1, 1) // sub-id 1 -> second extension
	for _, m := range ms {
		bw.put(1, int(m)+1) // FS for gamma
	}
	dst := make([]byte, 4)
	n, err := Decode(dst, bw.bytes(), cfg)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	for i := range want {
		if uint32(dst[:n][i]) != want[i] {
			t.Fatalf("sample %d = %d, want %d", i, dst[i], want[i])
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./aec/ -run 'TestZeroBlock|TestSecondExtension' -v`
Expected: FAIL — `lowEntropy` returns `ErrData`.

- [ ] **Step 3: Write the implementation**

In `aec/decoder.go` replace `func (d *decoder) lowEntropy(ref int) error { return ErrData }` with the three functions:

```go
// lowEntropy handles id==0: read a 1-bit sub-id, then (if ref) the reference
// sample, then dispatch to second extension (sub-id 1) or zero block (sub-id 0).
// Mirrors m_low_entropy + m_low_entropy_ref.
func (d *decoder) lowEntropy(ref int) error {
	sub, ok := d.br.getBits(1)
	if !ok {
		return ErrShortInput
	}
	if ref == 1 {
		v, ok := d.br.getBits(d.cfg.BitsPerSample)
		if !ok {
			return ErrShortInput
		}
		d.rsiBuf[d.rsip] = v
		d.rsip++
	}
	if sub == 1 {
		return d.secondExtension(ref)
	}
	return d.zeroBlock(ref)
}

// zeroBlock emits a run of zero samples. The FS value gives zero_blocks = fs+1
// with the ROS (remainder-of-segment, value 5) special case. Mirrors m_zero_block.
func (d *decoder) zeroBlock(ref int) error {
	fs, ok := d.br.getFS()
	if !ok {
		return ErrShortInput
	}
	const ros = 5
	zb := int(fs) + 1
	switch {
	case zb == ros:
		b := d.rsip / d.blockSize // completed blocks in this RSI (ref slot already counted)
		zb = min(d.rsi-b, 64-(b%64))
	case zb > ros:
		zb--
	}
	zeroSamples := zb*d.blockSize - ref
	if zeroSamples < 0 || d.rsip+zeroSamples > d.rsiSize {
		return ErrData
	}
	for i := 0; i < zeroSamples; i++ {
		d.rsiBuf[d.rsip] = 0
		d.rsip++
	}
	return nil
}

// secondExtension decodes block_size/2 gamma values into sample pairs via the
// SE inverse table. The loop starts at i=ref so the reference sample (already
// placed by lowEntropy) keeps the pair parity. Mirrors m_se.
func (d *decoder) secondExtension(ref int) error {
	i := ref
	for i < d.blockSize {
		m, ok := d.br.getFS()
		if !ok {
			return ErrShortInput
		}
		if m > seTableSize {
			return ErrData
		}
		d1 := int32(m) - int32(d.seTable[2*m+1])
		if i&1 == 0 {
			d.rsiBuf[d.rsip] = uint32(int32(d.seTable[2*m]) - d1)
			d.rsip++
			i++
		}
		d.rsiBuf[d.rsip] = uint32(d1)
		d.rsip++
		i++
	}
	return nil
}
```

> **ROS note:** `b = rsip / block_size` matches libaec's `RSI_USED_SIZE / block_size` because the reference sample (when `ref==1`) was already written by `lowEntropy` before `zeroBlock` runs, exactly as `m_low_entropy_ref`'s `copysample` precedes `m_zero_block`. `min` is the Go 1.21+ builtin.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./aec/ -run 'TestZeroBlock|TestSecondExtension' -v`
Then the whole pure-Go suite: `go test ./aec/ -v`
Expected: PASS (all tests).

- [ ] **Step 5: Commit**

```bash
git add aec/decoder.go aec/aec_test.go
git commit -m "aec: zero-block (+ROS) and second-extension options"
```

---

## Task 7: libaec fixture generator + frozen golden vectors

Generates `(config, samples, encoded bitstream)` vectors with libaec's encoder and decodes them with the pure-Go `Decode`, asserting the originals come back. The `-tags libaec` file generates and commits `aec/testdata/vectors.json`; the default test reads it.

**Files:**
- Create: `aec/libaec_test.go` (`//go:build libaec`)
- Create: `aec/vectors_test.go` (default build — reads frozen fixtures)
- Create (generated, committed): `aec/testdata/vectors.json`

**Interfaces:**
- Consumes: `aec.Decode`, `aec.Config`, all `Flags`.
- Produces: on-disk fixture format — a JSON array of `{name, bitsPerSample, blockSize, rsi, flags, samplesBase64, streamBase64}` (samples are the expected decoded bytes; stream is the AEC input). Test code in both files shares a `type vector struct{...}` declared in `vectors_test.go` (default build) so both builds see it.

- [ ] **Step 1: Write the fixture-reading test (fails: no fixtures yet)**

Create `aec/vectors_test.go`:

```go
package aec

import (
	"encoding/base64"
	"encoding/json"
	"os"
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
```

- [ ] **Step 2: Run to confirm it fails**

Run: `go test ./aec/ -run TestGoldenVectors`
Expected: FAIL — `testdata/vectors.json` does not exist.

- [ ] **Step 3: Write the generator (`//go:build libaec`)**

Create `aec/libaec_test.go`:

```go
//go:build libaec

package aec

// libaec_test.go links the system libaec (encoder + decoder) and is built only
// with `-tags libaec`. It (re)generates testdata/vectors.json and runs the
// byte-exact differential test (Task 8). Install libaec first (macOS:
// `brew install libaec`).

/*
#cgo darwin CFLAGS: -I/opt/homebrew/include
#cgo darwin LDFLAGS: -L/opt/homebrew/lib -laec
#cgo linux LDFLAGS: -laec
#include <stdlib.h>
#include <string.h>
#include <libaec.h>
*/
import "C"

import (
	"encoding/base64"
	"encoding/json"
	"math/rand"
	"os"
	"testing"
	"unsafe"
)

// aecEncode encodes raw sample bytes with libaec, returning the bitstream.
func aecEncode(t testing.TB, raw []byte, cfg Config) []byte {
	t.Helper()
	out := make([]byte, len(raw)+len(raw)/2+4096)
	var strm C.struct_aec_stream
	if len(raw) > 0 {
		strm.next_in = (*C.uchar)(unsafe.Pointer(&raw[0]))
	}
	strm.avail_in = C.size_t(len(raw))
	strm.next_out = (*C.uchar)(unsafe.Pointer(&out[0]))
	strm.avail_out = C.size_t(len(out))
	strm.bits_per_sample = C.uint(cfg.BitsPerSample)
	strm.block_size = C.uint(cfg.BlockSize)
	strm.rsi = C.uint(cfg.RSI)
	strm.flags = C.uint(cfg.Flags)
	if rc := C.aec_buffer_encode(&strm); rc != C.AEC_OK {
		t.Fatalf("aec_buffer_encode rc=%d", int(rc))
	}
	return out[:int(strm.total_out)]
}

// aecDecodeC decodes with libaec into a buffer of exactly outLen bytes.
func aecDecodeC(t testing.TB, stream []byte, outLen int, cfg Config) []byte {
	t.Helper()
	out := make([]byte, outLen)
	var strm C.struct_aec_stream
	if len(stream) > 0 {
		strm.next_in = (*C.uchar)(unsafe.Pointer(&stream[0]))
	}
	strm.avail_in = C.size_t(len(stream))
	strm.next_out = (*C.uchar)(unsafe.Pointer(&out[0]))
	strm.avail_out = C.size_t(outLen)
	strm.bits_per_sample = C.uint(cfg.BitsPerSample)
	strm.block_size = C.uint(cfg.BlockSize)
	strm.rsi = C.uint(cfg.RSI)
	strm.flags = C.uint(cfg.Flags)
	if rc := C.aec_buffer_decode(&strm); rc != C.AEC_OK {
		t.Fatalf("aec_buffer_decode rc=%d", int(rc))
	}
	return out
}

// sweepVectors enumerates the parameter sweep + per-option payloads.
func sweepVectors(t testing.TB) []vector {
	rng := rand.New(rand.NewSource(1))
	type combo struct {
		bps   int
		flags Flags
	}
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
		// (lossless ⇒ equals canonical input). Recording libaec's output rather
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
					add(name, bps, bs, rsi, fl, bs*rsi*3+7, func(i int) uint32 {
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

// TestGenerateVectors regenerates the frozen fixtures. Run explicitly:
//   go test -tags libaec ./aec/ -run TestGenerateVectors
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
```

Add the small shared helpers used by the generator to `vectors_test.go` (default build, so they exist without the tag):

```go
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
```

(Add `import "strconv"` to `vectors_test.go`.)

- [ ] **Step 4: Generate fixtures, then run the default golden test**

Run: `go test -tags libaec ./aec/ -run TestGenerateVectors -v`
Expected: PASS, logs `wrote N vectors`, creates `aec/testdata/vectors.json`.

Run: `go test ./aec/ -run TestGoldenVectors`
Expected: PASS — every frozen vector round-trips through the pure-Go decoder. If any subtest fails, the pure-Go decoder diverges from libaec; debug with the differential test in Task 8.

- [ ] **Step 5: Commit (including the generated fixtures)**

```bash
git add aec/libaec_test.go aec/vectors_test.go aec/testdata/vectors.json
git commit -m "aec: libaec fixture generator + frozen golden vectors"
```

---

## Task 8: Retained libaec differential test

Adds a byte-for-byte cross-check against libaec to the `-tags libaec` build — the authoritative "validated with libaec during testing" oracle, retained permanently.

**Files:**
- Modify: `aec/libaec_test.go`

**Interfaces:**
- Consumes: `aecEncode`, `aecDecodeC`, `sweepVectors` (Task 7); `aec.Decode`.

- [ ] **Step 1: Write the differential test**

Append to `aec/libaec_test.go`:

```go
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
```

- [ ] **Step 2: Run the differential test**

Run: `go test -tags libaec ./aec/ -run TestDifferentialLibaec`
Expected: PASS — pure-Go output equals libaec for every swept config. Any failure pinpoints the first differing byte and config; fix the corresponding option/preprocessing logic and re-run (and regenerate fixtures via Task 7 if needed).

- [ ] **Step 3: Confirm default build still green and CGo-free**

Run: `CGO_ENABLED=0 go test ./aec/`
Expected: PASS (golden vectors + unit tests, no libaec).

- [ ] **Step 4: Commit**

```bash
git add aec/libaec_test.go
git commit -m "aec: retained libaec byte-exact differential test"
```

---

## Task 9: Wire `aec` into the decode package; drop CCSDS CGo

**Files:**
- Create: `decode/ccsds_decode.go`
- Delete: `decode/ccsds_cgo.go`, `decode/ccsds_nocgo.go`
- Modify: `decode/ccsds.go` (remove `ErrCgoRequired` definition + its doc), `decode/errors.go` (add `ErrCgoRequired`)

**Interfaces:**
- Consumes: `aec.Decode`, `aec.Config`, `aec.Flags` constants. The existing `decode.CCSDS` calls `ccsdsDecode(input, output []byte, bitsPerSample int, blockSize, rsi, flags uint) error`.
- Produces: unconditional `func ccsdsDecode(input, output []byte, bitsPerSample int, blockSize, rsi, flags uint) error`. `ErrCgoRequired` now lives in `decode/errors.go`.

- [ ] **Step 1: Write the failing test**

There is no separate unit test for `ccsdsDecode` (it is covered end-to-end in Task 10). For this task, the "test" is that the package compiles without CGo and the existing pure-Go suite passes. First, delete the CGo pair and move the error, which will break the build until the adapter exists:

```bash
git rm decode/ccsds_cgo.go decode/ccsds_nocgo.go
```

- [ ] **Step 2: Run build to verify it fails**

Run: `go build ./decode/`
Expected: FAIL — `undefined: ccsdsDecode` in `decode/ccsds.go`.

- [ ] **Step 3: Write the implementation**

Move `ErrCgoRequired` into `decode/errors.go`. Replace `decode/errors.go` with:

```go
package decode

import "errors"

var ErrBadComplexStream = errors.New("decode: malformed complex-packed stream")

// ErrCgoRequired is returned by the JPEG2000 (5.40) decoder when the binary
// was built without CGo. JPEG2000 links the system libopenjp2; there is no
// pure-Go fallback. Build with CGO_ENABLED=1 (the Go default) and libopenjp2
// installed to decode template 5.40. (CCSDS, 5.42, is pure Go and needs no CGo.)
var ErrCgoRequired = errors.New("decode: this packing requires a CGo build with the corresponding system library installed")
```

Remove the `ErrCgoRequired` block from `decode/ccsds.go` (lines defining the `var` and its doc comment, and the now-stale paragraph in the `CCSDS` doc that says it "hands it to the system libaec"). In `decode/ccsds.go`, delete:

```go
// ErrCgoRequired is returned by the CCSDS / JPEG2000 decoders ... (the whole block)
var ErrCgoRequired = errors.New(...)
```

and update the `CCSDS` doc comment's final paragraph to:

```go
// The Section 7 payload is the raw CCSDS/AEC bitstream; this function decodes
// it with the pure-Go aec package (a port of libaec) — no CGo required.
```

Also drop the now-unused `"errors"` import from `decode/ccsds.go` if present (it is — used only by the deleted var). Verify with goimports/build.

Create `decode/ccsds_decode.go`:

```go
package decode

import (
	"fmt"

	"github.com/pspoerri/go-tiled-eccodes/aec"
)

// ccsdsDecode decodes the CCSDS/AEC Section-7 bitstream into output using the
// pure-Go aec package. output must be sized numPoints*bytesPerSample; the GRIB
// flag byte maps directly onto aec.Flags. Mirrors the old libaec call site.
func ccsdsDecode(input, output []byte, bitsPerSample int, blockSize, rsi, flags uint) error {
	if len(input) == 0 {
		return fmt.Errorf("decode: empty CCSDS input")
	}
	n, err := aec.Decode(output, input, aec.Config{
		BitsPerSample: bitsPerSample,
		BlockSize:     int(blockSize),
		RSI:           int(rsi),
		Flags:         aec.Flags(flags),
	})
	if err != nil {
		return fmt.Errorf("decode: CCSDS: %w", err)
	}
	if n != len(output) {
		return fmt.Errorf("decode: CCSDS produced %d bytes, expected %d", n, len(output))
	}
	return nil
}
```

- [ ] **Step 4: Run build + pure-Go suite to verify it passes**

Run: `CGO_ENABLED=0 go build ./... && CGO_ENABLED=0 go vet ./decode/ ./aec/`
Expected: builds clean, no vet errors.

Run: `go test ./aec/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add decode/ccsds_decode.go decode/ccsds.go decode/errors.go
git commit -m "decode: route CCSDS (5.42) through pure-Go aec, drop libaec CGo"
```

---

## Task 10: GRIB end-to-end tests run unconditionally

Move the CCSDS end-to-end tests out of the `//go:build cgo` file so they exercise the pure-Go path on every build, and drop the obsolete nocgo CCSDS assertion.

**Files:**
- Create: `ccsds_test.go` (no build tag)
- Modify: `ccsds_jpeg2000_test.go` (remove CCSDS tests; keep JPEG2000, still `//go:build cgo`)
- Modify: `ccsds_jpeg2000_nocgo_test.go` (remove CCSDS assertion; JPEG2000-only)

**Interfaces:**
- Consumes: `grib.Open`, `Message.DecodeFloat64`, `loadTestdata` (defined in `integration_test.go`, package `grib_test`). Fixtures `testdata/regular_ll_ccsds.grib2`, `testdata/icon-d2_t_2m_ccsds.grib2`, `testdata/icon-d2_t_2m.grib2`.

- [ ] **Step 1: Create the unconditional CCSDS test file**

Create `ccsds_test.go` (move `TestCCSDSConstantField` and `TestCCSDSICOND2` verbatim from `ccsds_jpeg2000_test.go`, minus the build tag):

```go
package grib_test

import (
	"math"
	"testing"

	grib "github.com/pspoerri/go-tiled-eccodes"
)

// TestCCSDSConstantField decodes a CCSDS-packed copy of the regular_ll fixture
// (template 5.42). Source value is 273.15 K everywhere (nbits=0 short-circuit).
func TestCCSDSConstantField(t *testing.T) {
	f, err := grib.Open(loadTestdata(t, "regular_ll_ccsds.grib2"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()

	m := f.Messages()[0]
	if h := m.Header(); h.DataTemplate != 42 || h.Ni != 16 || h.Nj != 31 {
		t.Fatalf("header = tmpl %d %dx%d, want 42 16x31", h.DataTemplate, h.Ni, h.Nj)
	}
	vals, err := m.DecodeFloat64(nil)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(vals) != 16*31 {
		t.Fatalf("len = %d, want 496", len(vals))
	}
	for i, v := range vals {
		if math.Abs(v-273.15) > 1e-3 {
			t.Fatalf("vals[%d] = %g, want 273.15", i, v)
		}
	}
}

// TestCCSDSICOND2 decodes a CCSDS-packed ICON-D2 t_2m forecast and cross-checks
// per-cell values against the simple-packed reference (≤ 0.05 K). This is the
// real exercise of the pure-Go AEC decoder.
func TestCCSDSICOND2(t *testing.T) {
	ref, err := grib.Open(loadTestdata(t, "icon-d2_t_2m.grib2"))
	if err != nil {
		t.Fatalf("open ref: %v", err)
	}
	defer ref.Close()
	got, err := grib.Open(loadTestdata(t, "icon-d2_t_2m_ccsds.grib2"))
	if err != nil {
		t.Fatalf("open ccsds: %v", err)
	}
	defer got.Close()

	rv, _ := ref.Messages()[0].DecodeFloat64(nil)
	gv, err := got.Messages()[0].DecodeFloat64(nil)
	if err != nil {
		t.Fatalf("decode ccsds: %v", err)
	}
	if len(rv) != len(gv) {
		t.Fatalf("len mismatch: ref=%d ccsds=%d", len(rv), len(gv))
	}
	maxDiff, mismatches := 0.0, 0
	for i := range rv {
		if math.IsNaN(rv[i]) && math.IsNaN(gv[i]) {
			continue
		}
		if math.IsNaN(rv[i]) != math.IsNaN(gv[i]) {
			mismatches++
			continue
		}
		if d := math.Abs(rv[i] - gv[i]); d > maxDiff {
			maxDiff = d
		}
	}
	t.Logf("ICON-D2 CCSDS roundtrip: %d points, max diff %.4f K, NaN mismatches %d", len(rv), maxDiff, mismatches)
	if mismatches != 0 {
		t.Fatalf("NaN mismatches: %d", mismatches)
	}
	if maxDiff > 0.05 {
		t.Fatalf("max diff = %.4f K, want ≤ 0.05", maxDiff)
	}
}
```

- [ ] **Step 2: Remove the CCSDS tests from the cgo file**

Edit `ccsds_jpeg2000_test.go`: delete `TestCCSDSConstantField` and `TestCCSDSICOND2` (now in `ccsds_test.go`). Keep the `//go:build cgo` tag and the two JPEG2000 tests (`TestJPEG2000ICOND2`, `TestJPEG2000ConstantField`). Remove the now-unused `grib` import only if it becomes unused (it is still used by the JPEG2000 tests, so keep it).

Edit `ccsds_jpeg2000_nocgo_test.go`: delete `TestCCSDSReturnsCgoRequiredWhenDisabled`; keep `TestJPEG2000ReturnsCgoRequiredWhenDisabled`. The file stays `//go:build !cgo`.

- [ ] **Step 3: Run the end-to-end CCSDS tests (pure-Go path)**

Run: `CGO_ENABLED=0 go test . -run 'TestCCSDS' -v`
Expected: PASS — `TestCCSDSConstantField` and `TestCCSDSICOND2` both green via pure Go. The ICON-D2 log line should show `max diff ≤ 0.05 K` and `0` NaN mismatches.

Run (full suite, both modes):
`CGO_ENABLED=0 go test ./...` and `CGO_ENABLED=1 go test ./...`
Expected: PASS in both.

- [ ] **Step 4: Commit**

```bash
git add ccsds_test.go ccsds_jpeg2000_test.go ccsds_jpeg2000_nocgo_test.go
git commit -m "test: run CCSDS end-to-end tests unconditionally (pure-Go path)"
```

---

## Task 11: Benchmarks and optimization pass

Establish benchmarks, confirm 0 allocs/op, profile, and optimize the hot path while keeping all vectors green.

**Files:**
- Create: `aec/aec_bench_test.go`
- Modify: `bench_test.go` (add CCSDS case)
- Modify: `aec/libaec_test.go` (add libaec baseline benchmark)
- Possibly modify: `aec/bitreader.go`, `aec/decoder.go` (optimizations)

**Interfaces:**
- Consumes: `loadVectors` (Task 7), `aec.Decode`.

- [ ] **Step 1: Write the pure-Go benchmarks**

Create `aec/aec_bench_test.go`:

```go
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
```

- [ ] **Step 2: Run the benchmarks and confirm 0 allocs/op**

Run: `go test ./aec/ -run xxx -bench . -benchmem`
Expected: each benchmark reports `0 B/op	0 allocs/op` in the steady state. If allocs > 0, the `rsiBuf`/`seTable` are being reallocated per call — they are built in `newDecoder`, so per-`Decode` there is one `decoder` alloc + two slices. To hit 0 allocs/op on a hot caller path, document that the per-`Decode` setup allocates `O(rsiSize)` once; the *steady-state per-sample loop* must allocate nothing (verify the profile shows no allocs inside `run`/`flush`/option methods). Record the baseline numbers in the commit message.

> Acceptance for this step: no allocations occur inside the decode loop (`run`, option methods, `flush`, `put`). The one-time `newDecoder` allocations (decoder struct, `rsiBuf`, `seTable`) are acceptable and counted once per `Decode` call.

- [ ] **Step 3: Add the libaec baseline benchmark (`-tags libaec`)**

Append to `aec/libaec_test.go`:

```go
// BenchmarkDecodeLibaecBaseline decodes the same ramp vector with libaec, for a
// throughput comparison against the pure-Go BenchmarkDecodeRamp. -tags libaec only.
func BenchmarkDecodeLibaecBaseline(b *testing.B) {
	vs := loadVectors(b)
	var stream, want []byte
	var cfg Config
	for _, v := range vs {
		if containsStr(v.Name, "ramp") {
			want, _ = base64.StdEncoding.DecodeString(v.SamplesB64)
			stream, _ = base64.StdEncoding.DecodeString(v.StreamB64)
			cfg = Config{BitsPerSample: v.BitsPerSample, BlockSize: v.BlockSize, RSI: v.RSI, Flags: Flags(v.Flags)}
			break
		}
	}
	b.SetBytes(int64(len(want)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = aecDecodeC(b, stream, len(want), cfg)
	}
}
```

Run: `go test -tags libaec ./aec/ -run xxx -bench 'Ramp|Libaec' -benchmem`
Record both numbers. Target: pure-Go within parity of, or faster than, libaec.

- [ ] **Step 4: Add the GRIB-level CCSDS benchmark**

In `bench_test.go`, add (mirroring `BenchmarkDecodeICONComplex`):

```go
func BenchmarkDecodeICONCCSDS(b *testing.B) {
	path := loadTestdata(b, "icon-d2_t_2m_ccsds.grib2")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f, _ := grib.Open(path)
		if _, err := f.Messages()[0].DecodeFloat64(nil); err != nil {
			b.Fatal(err)
		}
		f.Close()
	}
}
```

Run: `go test . -run xxx -bench BenchmarkDecodeICON -benchmem`
Expected: the CCSDS case runs; record ns/op alongside the other packings.

- [ ] **Step 5: Optimize (only if profiling shows headroom), re-verify, commit**

If `BenchmarkDecodeRamp` is materially slower than libaec, profile:
`go test ./aec/ -run xxx -bench BenchmarkDecodeRamp -cpuprofile cpu.out && go tool pprof -top cpu.out`

Apply the techniques from the spec, smallest-change-first, re-running `go test ./aec/` (and `-tags libaec ./aec/ -run TestDifferentialLibaec`) after each change so correctness never regresses:
- Replace the bit-by-bit `getFS` loop with a `math/bits.LeadingZeros64`-based scan over the accumulator window (mask to `cnt` bits, find the top set bit). Keep `getBits`/`getFS` results identical.
- Hoist `bytesPerSample`/`msb`/`k` out of per-sample loops; ensure `put` is inlined.
- Add a length hint (`_ = d.dst[d.outPos+d.bytesPerSample-1]`) before width-specialized writes if it removes bounds checks (verify with `go build -gcflags=-d=ssa/check_bce`).

After optimization:
Run: `go test ./aec/ && go test -tags libaec ./aec/ -run TestDifferentialLibaec`
Expected: PASS. Re-run benchmarks; the differential test guarantees byte-exactness held.

```bash
git add aec/aec_bench_test.go aec/libaec_test.go aec/bitreader.go aec/decoder.go bench_test.go
git commit -m "aec: benchmarks, 0-alloc hot path, optimization pass"
```

---

## Task 12: Documentation + final verification

**Files:**
- Modify: `README.md`
- Modify: spec implementation-notes (record perf numbers + any upstream findings)

**Interfaces:** none (docs only).

- [ ] **Step 1: Update README prose**

In `README.md`, change the intro paragraph that reads:

> "JPEG2000 (5.40) and CCSDS (5.42) link `libopenjp2` and `libaec` via CGo, so a pure-Go `go build` still produces a self-contained binary for archives that don't use those two packings."

to name only JPEG2000:

> "JPEG2000 (5.40) links `libopenjp2` via CGo behind a build tag; every other packing — including CCSDS (5.42), now a pure-Go port of libaec — is pure Go, so a pure-Go `go build` decodes everything except JPEG2000."

Update the "Optional packings" / feature bullets similarly: CCSDS is no longer behind a CGo tag. Update the "Seven packing decoders" sentence so CCSDS is listed among the pure-Go set and only JPEG2000 remains CGo.

- [ ] **Step 2: Update the Section 5 table**

In the `### Data Representation Templates (Section 5)` table, change the CCSDS row:

```
| 5.42 | CCSDS                                           | ✅ — pure Go (port of libaec) |
```

(JPEG2000 row 5.40 stays `✅ — via libopenjp2 (CGo)`.)

- [ ] **Step 3: Record implementation notes in the spec**

Append an `## Implementation notes` section to `docs/superpowers/specs/2026-06-17-ccsds-libaec-pure-go-port-design.md` with: the measured pure-Go vs libaec throughput on the ramp vector and the ICON-D2 GRIB benchmark; confirmation of 0 allocs/op in the decode loop; and any upstream libaec findings (per the spec's "Reporting upstream findings" section — if none, state "no bugs found; port is bug-compatible with libaec v1.1.7").

- [ ] **Step 4: Full verification**

Run all of:
```bash
gofmt -l aec/ decode/ . | grep . && echo "FORMAT ISSUES" || echo "gofmt clean"
CGO_ENABLED=0 go build ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=1 go test ./...
go test -tags libaec ./aec/ -run 'TestDifferentialLibaec|TestGoldenVectors'
```
Expected: gofmt clean; all builds and tests PASS in pure-Go and cgo modes; the differential test passes against libaec.

- [ ] **Step 5: Commit**

```bash
git add README.md docs/superpowers/specs/2026-06-17-ccsds-libaec-pure-go-port-design.md
git commit -m "docs: CCSDS (5.42) is now pure Go; record perf + findings"
```

---

## Self-Review (completed by plan author)

**Spec coverage:**
- Public `aec` package + API (Flags/Config/Decode) → Task 1. ✅
- MSB-first bit reader → Task 2. ✅
- SE inverse table → Task 3. ✅
- Four code options (split/zero+ROS/second-extension/uncompressed) → Tasks 4–6. ✅
- Preprocessing reversal (libaec zig-zag+clamp, signed/unsigned, sign-extended reference) + output serialization (width/endianness, 3-byte honors MSB) → Task 4. ✅
- Drop CCSDS CGo, keep `ErrCgoRequired` for JPEG2000, adapter in `decode` → Task 9. ✅
- Frozen golden vectors (no libaec at runtime) → Task 7; retained `//go:build libaec` differential test → Task 8. ✅
- End-to-end GRIB tests unconditional; nocgo test trimmed → Task 10. ✅
- Performance: benchmarks, 0-alloc hot path, libaec baseline, optimization → Task 11. ✅
- Docs + upstream-findings note → Task 12. ✅

**Placeholder scan:** No TBD/TODO; every code step has complete code. The only deliberate temporary stubs (`split`/`lowEntropy` returning `ErrData`) are replaced in named later tasks with full code.

**Type consistency:** `decoder` fields (`idLen`, `idMax`, `bytesPerSample`, `rsiBuf`, `rsip`, `needed`, `emitted`, `lastOut`, `msb`) are declared in Task 1/4 and used consistently; method set (`run`, `decodeBlock`, `uncomp`, `split`, `lowEntropy`, `zeroBlock`, `secondExtension`, `flush`, `put`) is stable across tasks; `vector` struct and helpers (`bytesPerSampleFor`, `writeSample`, `maskBits`, `vecName`) are shared via the default-build `vectors_test.go` so the `-tags libaec` file compiles. `ccsdsDecode` signature is unchanged from the deleted CGo version, so `decode.CCSDS`'s call site needs no edit.
