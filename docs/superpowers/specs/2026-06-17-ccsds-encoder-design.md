# CCSDS (DRT 5.42) Encoding in the `writer` Package

**Date:** 2026-06-17
**Status:** Approved (scope: writer parity)

## Motivation

On ~2026-06-16 DWD switched the packing of its ICON open-data products from
simple packing (Data Representation Template 5.0) to CCSDS adaptive entropy
coding (DRT 5.42, CCSDS 121.0-B, the algorithm `libaec` implements). Verified
against a freshly downloaded ICON-D2 `t_2m` file:

```
[0] disc=0 cat=0 num=0 ref=2026-06-17T00:00Z fcst=0 gridT=101 packT=42 N=542040
```

The **decoder already handles 5.42** end-to-end — the live file decodes to
542,040 points (525,072 valid, 16,968 NaN, range 270–300 K). The gap is the
**encoder**: the `writer` package can emit 5.0 / 5.4 (IEEE) / 5.41 (PNG) but
not 5.42. This work adds writer parity so synthetic fixtures and round-trip
tests can exercise the format DWD now ships.

## Scope

In scope:
- A `PackingCCSDS` option in `writer` that emits Section 5 template 5.42 and a
  CCSDS-compressed Section 7.
- A cgo-backed AEC encoder mirroring the existing cgo-backed AEC decoder, with
  a nocgo stub.
- Round-trip and nocgo tests.

Out of scope (explicitly deferred):
- A general transcode CLI that re-packs arbitrary real GRIB2 → 5.42.
- A pure-Go AEC encoder (libaec is thousands of lines of careful C; the
  codebase already delegates AEC decode to libaec for the same reason).

## Background: how CCSDS relates to simple packing

CCSDS packing is identical to simple packing up to and including the integer
quantization step. Both compute, per value `Y`:

```
X = round( (Y - R) / 2^E / 10^D )      (0 ≤ X < 2^nbits)
```

The existing `writer.pack()` already produces `R, E, D, nbits` and the integer
samples. The two formats then diverge:

| | Simple (5.0) | CCSDS (5.42) |
|---|---|---|
| Section 5 body | `R(4) E(2) D(2) nbits(1) type(1)` = 10 B | the same 10 B **+** `flags(1) blockSize(1) rsi(2)` = 14 B |
| Section 7 | raw bit-packed `nbits`-wide samples | samples serialized to `bytesPerSample`-wide MSB-first words, then `aec_buffer_encode` |

### Section 5 template 5.42 layout (template bytes, from S5 byte 11)

```
0-3   reference value R   (IEEE-754 float32 BE)
4-5   binary scale E      (sign-magnitude int16)
6-7   decimal scale D     (sign-magnitude int16)
8     bits per value      (nbits)
9     type of original field values (0 = float)
10    CCSDS flags  (libaec flag mask)
11    CCSDS block size
12-13 CCSDS reference sample interval (uint16 BE)
```

This mirrors `decode.CCSDS` exactly (the inverse already exists).

### DWD's parameters (verified from the live file)

Real DWD Section 5 template bytes: `43 87 0d d0 80 0b 00 00 10 00 0e 20 00 80`
- `flags = 14` = `AEC_DATA_3BYTE(2) | AEC_DATA_MSB(4) | AEC_DATA_PREPROCESS(8)`
- `blockSize = 32`
- `rsi = 128`
- `nbits = 16`, `type = 0`, unsigned (no `AEC_DATA_SIGNED`)

These become the writer defaults so writer output matches DWD's byte layout.

## Section 7 sample serialization

`bytesPerSample` is chosen exactly as the decoder chooses it
(`bytesPerSampleFromBits`): 1 byte for nbits ≤ 8, 2 for ≤ 16, 3 if
`AEC_DATA_3BYTE` and ≤ 24, else 4. Samples are unsigned (X ≥ 0) and written
MSB-first when `AEC_DATA_MSB` is set (it is, by default) — the exact inverse of
`decode.CCSDS`'s read loop. The serialized buffer is the AEC encoder input;
`bits_per_sample = nbits`.

The realistic ICON path is nbits ≤ 16 (1–2 byte samples). The encoder
implements 1/2/3/4-byte + MSB/LSB symmetrically with the decoder for
correctness, even though only ≤16-bit is exercised by ICON.

## Components

### 1. `writer.Field` additions

```go
Packing PackingType  // gains PackingCCSDS

// Optional CCSDS tuning; zero values default to DWD's parameters.
CCSDSFlags     uint8   // default 14
CCSDSBlockSize uint8   // default 32
CCSDSRSI       uint16  // default 128
```

`PackingCCSDS` is appended to the `PackingType` iota after `PackingPNG`.

### 2. `writer.encodeDataCCSDS(valid []float64, nset int, f Field) (s5, s7 []byte, err error)`

- Calls `pack(valid, f.NumBits)` for `R, E, D, nbits` and the integer samples.
- Constant field (`nbits == 0`): emit Section 5 with nbits 0 and an **empty**
  Section 7 (matches what `decode.CCSDS` expects — it short-circuits on
  nbits 0 and never calls libaec). No AEC call.
- Otherwise serialize samples → call `ccsdsEncode` → build S5 (14-byte body)
  and S7.

### 3. cgo split (mirrors `decode/ccsds_cgo.go`)

- `writer/ccsds_cgo.go` (`//go:build cgo`): `ccsdsEncode(input []byte,
  bitsPerSample int, blockSize, rsi, flags uint) ([]byte, error)` wrapping
  `aec_buffer_encode`. Output buffer sized with headroom
  (`len(input) + len(input)/2 + 64`), trimmed to `strm.total_out`. Pins
  input/output per cgo pointer rules, same as the decoder.
- `writer/ccsds_nocgo.go` (`//go:build !cgo`): stub returning a sentinel
  `ErrCCSDSNeedsCgo` (writer-local, `errors.Is`-able).

### 4. Plumbing

`encodeData` currently returns `(s5, s6, s7 []byte)`. CCSDS can fail (nocgo
stub, libaec error) where simple/IEEE/PNG cannot, so `encodeData` gains an
`error` return and `encodeMessage` propagates it. The S6 bitmap logic is
unchanged and shared.

## Error handling

- Pure-Go (`CGO_ENABLED=0`) build: compiles fine; `Single`/`Bundle`/
  `EncodeFile` with `PackingCCSDS` return `ErrCCSDSNeedsCgo`. Consistent with
  the decoder returning `decode.ErrCgoRequired`.
- libaec failure: wrapped error `"writer: libaec aec_buffer_encode rc=%d"`.

## Testing

`writer/ccsds_test.go` (`//go:build cgo`):
- **Linear field round-trip**: writer 5.42 → grib decoder, values within
  simple-packing precision (≈0.01 over the field span); assert
  `DataTemplate == 42`.
- **Constant field**: `NumBits = 0`, assert empty Section 7 path and exact
  reconstruction.
- **Bitmap/NaN field**: NaNs survive the round-trip (Section 6 path is shared,
  but exercise it under CCSDS).
- **Section 5 byte-shape**: assert the 14-byte template body has
  `flags=14, blockSize=32, rsi=128, nbits=16, type=0` — the DWD layout.
- **Override**: a non-default `CCSDSBlockSize`/`CCSDSRSI` still round-trips.

`writer/ccsds_nocgo_test.go` (`//go:build !cgo`): `PackingCCSDS` returns a
`ErrCCSDSNeedsCgo`-matching error.

Verification commands:
```
CGO_ENABLED=1 go test ./writer/ ./...
CGO_ENABLED=0 go vet ./...   # pure-Go still builds
CGO_ENABLED=0 go test ./writer/ -run CgoRequired
```

## Risks

- **MSB ordering / sample width drift** between encode and decode. Mitigated by
  implementing encode as the literal inverse of `decode.CCSDS` and round-trip
  testing.
- **AEC output-buffer overflow** for incompressible data. Mitigated by sizing
  output with 50% + 64-byte headroom and checking `total_out`; AEC growth over
  raw input is bounded and small.
