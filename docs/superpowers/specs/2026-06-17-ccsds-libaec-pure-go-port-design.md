# Pure-Go CCSDS (5.42) Decoder — libaec Port

**Date:** 2026-06-17
**Status:** Approved (design)
**Scope:** Decoder only. No encoder, no streaming/incremental decode.

## Goal

Port libaec's adaptive entropy decoder (CCSDS 121.0-B-3) to pure Go and make
it the *only* CCSDS path, so GRIB2 Data Representation Template 5.42 decodes
without CGo. After this change a pure-Go `go build` produces a self-contained
binary that decodes CCSDS-packed messages; the `-laec` system-library
dependency is removed entirely.

The decoder is exposed as a **public, reusable package** so consumer
libraries can decode AEC/CCSDS bitstreams directly, independent of GRIB.

The pure-Go decoder must be **rigorously benchmarked and optimized** — at
least competitive with libaec, and allocation-free in the steady-state decode
loop. See the Performance section.

JPEG2000 (5.40) is unaffected — it still links `libopenjp2` via CGo and still
returns `ErrCgoRequired` on pure-Go builds.

## Non-goals

- AEC **encoding** (`aec_buffer_encode`). The library is read-side only; the
  `writer` subpackage emits only simple/IEEE/PNG packings.
- Streaming / incremental decode (libaec's `aec_decode` with retained state).
  Only whole-buffer decode (`aec_buffer_decode`) is in scope. The package is
  structured so either could be added later without breaking the API.
- Changes to JPEG2000 (5.40) handling.

## Module structure & public API

New package `github.com/pspoerri/go-tiled-eccodes/aec` (`./aec/`):

```go
package aec

type Flags uint

const (
    DataSigned      Flags = 1 << iota // AEC_DATA_SIGNED:     samples are signed
    Data3Byte                         // AEC_DATA_3BYTE:      24-bit storage for 17–24 bps
    DataMSB                           // AEC_DATA_MSB:         big-endian output bytes
    DataPreprocess                    // AEC_DATA_PREPROCESS:  predictor/mapping applied
    RestrictedCodes                   // AEC_RESTRICTED:       restricted code-option set
    PadRSI                            // AEC_PAD_RSI:          RSI padded to byte boundary
)

type Config struct {
    BitsPerSample int   // 1..32
    BlockSize     int   // 8, 16, 32, or 64
    RSI           int   // reference sample interval, in blocks per segment
    Flags         Flags
}

// Decode decodes the AEC bitstream src into dst, writing BitsPerSample-wide
// samples in the byte layout libaec produces — storage width (1/2/3/4 bytes)
// per BitsPerSample and Data3Byte, endianness big iff DataMSB (the 3-byte
// form is always big-endian). It returns the number of bytes written,
// mirroring aec_buffer_decode's total_out. dst must be large enough for all
// decoded samples; an undersized dst is an error.
func Decode(dst, src []byte, cfg Config) (int, error)
```

The flag bit positions match libaec's `<libaec.h>` values exactly
(`DataSigned`=0x01 … `PadRSI`=0x20), so the GRIB byte-11 options mask maps
directly onto `Flags`.

### Integration with the `decode` package

- `decode/ccsds.go` is unchanged except for `ccsdsDecode`. Its byte→float64
  post-processing (reference value, binary/decimal scale, sign extension,
  endianness per the MSB flag, 3-byte handling) is already validated against
  libaec by the current CGo test, so keeping the byte-buffer boundary keeps
  the blast radius minimal.
- `ccsdsDecode` becomes a thin, unconditional adapter: it maps the GRIB flag
  byte and parameters to an `aec.Config` and calls `aec.Decode`. No build
  tags.
- `decode/ccsds_cgo.go` and `decode/ccsds_nocgo.go` are **deleted**. The
  `#cgo … -laec` directive and the `ccsdsDecode` cgo/nocgo split go away.
- `ErrCgoRequired` **stays** in `decode/ccsds.go` (or moves to the JPEG2000
  files) — JPEG2000 still needs it. The doc comment is updated so it no
  longer claims CCSDS requires CGo.

## Decoder internals

A `Decoder` struct wraps an MSB-first `bitReader` over `src` and mirrors
libaec's `decode.c` state machine in idiomatic Go.

### Initialization

Computed once from `Config`:

- `idLen` — option-ID field width in bits (1–5), derived from
  `BitsPerSample` and `RestrictedCodes`, matching libaec's init logic.
  `idMax = (1 << idLen) - 1`.
- `bytesPerSample` — output storage width: 1 (bps ≤ 8), 2 (≤ 16),
  3 (`Data3Byte` and 17–24), else 4.
- `xmin`/`xmax` — representable sample range: unsigned `[0, 2^bps-1]`,
  signed `[-2^(bps-1), 2^(bps-1)-1]`.

### Per-segment / per-block decode

Decoding runs per **RSI segment** (`RSI` blocks × `BlockSize` samples, with
the final segment possibly short). For each block, read the `idLen`-bit
option ID and dispatch to one of four code options. These are the discrete,
independently testable units:

1. **k-split (sample splitting)** — `1 ≤ id ≤ idMax-1`, split `k = id - 1`.
   Read `BlockSize` unary fundamental-sequence values (the high parts), then
   `BlockSize × k` literal low bits; each sample = `(fs << k) | low`. `k = 0`
   is the pure fundamental-sequence case (no remainder bits).
2. **Zero-block** — `id == 0`, selected by the low-entropy sub-id. Read a
   unary run length and emit that many all-zero blocks. Handle the
   **remainder-of-segment (ROS)** special count, which fills the rest of the
   current segment with zero blocks.
3. **Second extension** — `id == 0`, the other low-entropy sub-id. Read
   `BlockSize/2` unary γ values; invert the triangular map
   `γ = (d₀+d₁)(d₀+d₁+1)/2 + d₁` to recover each `(d₀, d₁)` pair.
4. **Uncompressed** — `id == idMax`. Read `BlockSize` literal
   `BitsPerSample`-bit samples verbatim.

### Preprocessing reversal

When `DataPreprocess` is set (the GRIB case):

- The **first sample of each RSI segment is a reference**: read as a raw
  `BitsPerSample` literal and output directly. It seeds the predictor for the
  rest of the segment; the remainder of its block decodes as differences.
- Every subsequent mapped value `d` is inverted to a signed delta via the
  boundary-aware unmapping, then accumulated onto the predictor `prev`:
  - `theta = min(prev - xmin, xmax - prev)`
  - if `d <= 2*theta`: `delta = d/2` when `d` even, else `delta = -(d+1)/2`
  - else: `delta = d - theta` if the lower boundary is closer
    (`theta == prev - xmin`), else `delta = theta - d`
  - `x = prev + delta`, then `prev = x`.

When `DataPreprocess` is off, the decoded option values are the samples
directly (unsigned, reinterpreted as signed when `DataSigned`).

### Output serialization

Each reconstructed sample is written at `bytesPerSample` width, big-endian
iff `DataMSB` (3-byte form always big-endian). This reproduces exactly what
libaec's `aec_buffer_decode` writes, so `decode/ccsds.go`'s existing
byte→float64 conversion needs no change.

## Testing & validation

Three layers, ordered by what they catch:

### 1. Frozen golden vectors (`aec/testdata/`) — byte-exact, primary net

AEC is lossless, so the truth for any encoded bitstream is the original
sample array. A dev-time generator uses libaec's **encoder** to produce
`(random samples → encoded bitstream)` pairs, and writes the bitstream plus
the expected decoded bytes into checked-in fixtures.

- The generator is build-tagged `//go:build aecgen` (and links libaec via
  CGo), so normal builds and `go test ./...` never touch libaec.
- Parameter sweep covers: `BitsPerSample` boundaries (1, 8, 9, 16, 17, 24,
  25, 32), each `BlockSize` (8/16/32/64), small and large `RSI`, signed and
  unsigned, `DataMSB` on/off, `Data3Byte`, `DataPreprocess` on/off, and
  payloads shaped to force each code option: all-zero runs (zero-block / ROS),
  low-variance data (second extension), high-entropy random (uncompressed),
  and mid-range data (k-split across several k).
- Default `go test ./aec` reads only the frozen fixtures and asserts the
  pure-Go decoder reproduces the original samples exactly. **No libaec at
  test runtime.**

### 2. Unit tests on the fiddly primitives

Hand-constructed cases for the parts most prone to off-by-one and boundary
bugs: the MSB-first `bitReader` (cross-byte unary runs, literal reads
straddling byte boundaries, end-of-stream) and the second-extension
triangular inverse.

### 3. End-to-end GRIB tests

- `TestCCSDSConstantField` (nbits=0 constant field) and `TestCCSDSICOND2`
  (real DWD ICON-D2 `t_2m`, cross-checked ≤ 0.05 K against the simple-packed
  reference fixture) move from `//go:build cgo` to **unconditional**. They now
  exercise the pure-Go path. Note that the constant-field fixture has nbits=0
  and is short-circuited in `ccsds.go` before the AEC decoder runs, so
  `TestCCSDSICOND2` is the only end-to-end test that exercises real AEC
  decoding — hence the golden vectors carry the coverage load.
- `TestCCSDSReturnsCgoRequiredWhenDisabled` is updated to drop the CCSDS
  assertion (CCSDS no longer needs CGo) and keep only the JPEG2000 case. Its
  `//go:build !cgo` file is renamed/retitled accordingly.

### Development process

TDD against the golden vectors. During the port a temporary CGo differential
check (decode the same bitstream with both pure-Go and libaec, assert
byte-equality) may be used to localize any mismatch; it is removed from the
shipped tree before completion.

## Performance

The decoder must be fast and allocation-free in steady state. Order of work:
**correct first** (pass all golden vectors), **then profile and optimize**,
re-running the vector suite after every optimization so speed never trades
away correctness.

### Benchmarks (`aec/aec_bench_test.go`)

- Decode-throughput benchmarks over representative inputs: the real ICON-D2
  `t_2m` Section-7 payload, plus synthetic large arrays exercising each code
  option (zero-block, second extension, k-split, uncompressed) and each
  `BitsPerSample` storage width (1/2/3/4 byte). Report MB/s and ns/sample via
  `b.SetBytes`.
- `b.ReportAllocs()` on every benchmark; the steady-state target is **0
  allocs/op** for a caller-supplied `dst` (the decoder allocates no
  per-block/per-segment scratch on the hot path).
- A baseline comparison against libaec (CGo, build-tagged `//go:build aecgen`
  alongside the vector generator) is recorded **once** during development to
  confirm the pure-Go decoder is at least competitive — ideally faster, since
  it avoids the cgo call boundary and pointer pinning. The comparison is not
  part of the shipped test suite.
- The repo-level `bench_test.go` / `eccodestest` GRIB benchmarks gain a CCSDS
  case so end-to-end first-tile decode cost is tracked.

### Optimization targets & techniques

- **Bit reader:** a 64-bit accumulator with bulk refill rather than bit-at-a-
  time reads; decode unary fundamental-sequence runs with
  `math/bits.LeadingZeros64` instead of per-bit loops; slice the input so the
  compiler can elide bounds checks on the hot path.
- **Inner loops:** keep the k-split FS/remainder loops branch-lean; hoist
  invariants (`k`, `bytesPerSample`, masks) out of the per-sample loop;
  specialize output serialization per `bytesPerSample` width.
- **No hidden allocation:** reuse `dst`; any decoder scratch lives in the
  `Decoder` struct, sized once at init.
- Profile with `go test -bench` + pprof CPU profiles; iterate on the hottest
  frames. Record before/after numbers in the implementation notes.

### Acceptance criteria

- 0 allocs/op on the steady-state decode path (caller-supplied `dst`).
- Decode throughput within parity of — or better than — libaec on the
  ICON-D2 payload, per the one-time dev baseline.
- All golden vectors and end-to-end GRIB tests still pass after optimization.

## Documentation

`README.md` Section 5 table and the "Optional packings" / intro prose are
updated: CCSDS (5.42) moves to pure Go; only JPEG2000 (5.40) remains behind
the CGo build tag. The opening paragraph ("JPEG2000 (5.40) and CCSDS (5.42)
link `libopenjp2` and `libaec` via CGo") is corrected to name only JPEG2000.

## Risks & mitigations

- **Subtle bitstream bugs** (unary across byte boundaries, ROS, SE pairing,
  per-RSI reference, restricted code set). Mitigation: faithful port of
  libaec's logic plus byte-exact golden vectors across the full parameter
  sweep.
- **Output-layout mismatch with `ccsds.go`.** Mitigation: `aec.Decode`
  reproduces libaec's exact output byte layout; the unchanged `ccsds.go`
  post-processing is already validated against that layout by the current
  CGo test, which is retained end-to-end.
- **libaec availability for vector generation.** The generator needs libaec
  once, at dev time, to produce fixtures. Mitigation: fixtures are checked in;
  the regular test suite has no libaec dependency.
```
