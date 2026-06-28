# go-tiled-ECCODES

GRIB2 decoder built for one job: serving WGS84 (XYZ) tiles out of mmapped
weather files at low latency. A drop-in replacement for the read side of
ECMWF [eccodes](https://github.com/ecmwf/eccodes). The common packings
(simple, complex, spatial differencing, IEEE float, PNG) and every grid
type are pure Go; JPEG2000 (5.40) links `libopenjp2` via CGo behind a
build tag; every other packing — including CCSDS (5.42), now a pure-Go
port of libaec — is pure Go, so a pure-Go `go build` decodes everything
except JPEG2000.

## Why

`eccodes` is the reference GRIB implementation and covers the full
spectrum of meteorological I/O. A tile server only needs a slice of
that: load a forecast grid once, project lat/lon → grid coordinates,
resample 65k pixels into a 256² tile, encode, ship. Specializing for
that path lets us keep the common case in pure Go (easier builds and
cross-compilation) and lean on Go's concurrency model — a `*grib.File`
is immutable after `Open`, so the same message can be rendered from
many goroutines without locking. This library targets exactly that path:

- **Static binary, single-file deployment by default.** Pure-Go `go build`
  produces a self-contained binary that runs anywhere Go runs, decoding
  all packings except JPEG2000 (5.40) — CCSDS (5.42) is now a pure-Go
  port of libaec and needs no system library. Only JPEG2000 delegates to
  `libopenjp2` via CGo; see *Optional packings* below.
- **mmap by default.** The OS owns the page cache; you don't pay for
  reads on hot tiles. Decoded grids are cached per-message in a
  `sync.Once` so the first tile pays the decode cost once and every
  subsequent tile is pure resampling.
- **Goroutine-safe by construction.** A `*grib.File` and its
  `*grib.Message`s are immutable after `Open`; render N tiles
  concurrently from the same message with no locking on the decode path.
- **Hot loop is allocation-free.** Tile working buffers (fx/fy and
  scratch float64) are pooled by size bucket. A 256² tile render
  allocates ~6 KB total.

## Features

- **GRIB2 reader** with mmap, lazy message indexing, and
  one-decode-per-message caching.
- **Seven packing decoders**: simple (5.0), complex (5.2), complex +
  spatial differencing (5.3), IEEE float (5.4), PNG (5.41), CCSDS (5.42,
  pure-Go port of libaec) — all pure Go — plus JPEG2000 (5.40) via
  libopenjp2 (CGo, behind a build tag).
- **Seven grid types**: regular lat/lon (3.0), rotated lat/lon (3.1),
  Mercator (3.10), polar stereographic (3.20), Lambert conformal
  (3.30), Gaussian regular & reduced (3.40 with `pl[]` table), and
  general unstructured / icosahedral (3.101) for ICON-style triangular
  meshes. Gaussian latitudes are computed via Newton iteration on the
  Legendre polynomial; unstructured grids use a balanced 3-D KD-tree
  over unit-sphere chord distance and accept a `MaxDistance` footprint
  cap to mask the area outside a regional model's domain.
- **WGS84 XYZ tile API.** Slippy-map / OSM-convention tile
  coordinates → spherical-Mercator bbox → per-pixel lat/lon →
  resampled output. The renderer handles any grid type uniformly
  through a single `Grid` interface.
- **Three samplers**: nearest, bicubic (Catmull-Rom), and a
  histogram-based mode filter for categorical fields. Bicubic
  degrades gracefully to nearest at any NaN border so missing-data
  pixels don't bleed.
- **Ten output buffer types**: `float32`, `float64`, plus all eight
  signed and unsigned integer widths (`int8/16/32/64`,
  `uint8/16/32/64`). Integer renderers apply a user-supplied
  `Quantize` (scale + offset + clamp + missing-value sentinel).
- **Single-point lookup** (`ValueAt`) shares the same decode cache
  and sampler — useful for "what's the temperature at this airport"
  endpoints alongside the tile API. ~88 ns warm.
- **Bitmap (Section 6)** handling: missing values surface as `NaN`
  in the float64 buffer and as `MissingValue` in integer outputs.
- **Two CLI tools** (`grib-inspect`, `grib-tile`) for smoke testing
  against real fixtures.
- **Real-world validated.** Round-trip-tested against a live DWD
  ICON-D2 t_2m forecast — 754,862 valid points match `grib_dump`
  exactly across the simple, complex, and spatial-differencing
  encodings of the same field.

## Scope

| Feature | Status |
|---|---|
| GRIB2 reader (mmap, lazy index) | ✅ |
| GRIB1 | ❌ out of scope |
| BUFR | ❌ out of scope |
| Encode / write | ⚠️ partial — `writer` subpackage emits simple (5.0), IEEE float (5.4), and PNG (5.41) packings on rectangular grids (3.0, 3.1, 3.10, 3.20, 3.30, regular 3.40) for tests and synthetic fixtures; not a full GRIB2 producer |

### Data Representation Templates (Section 5)
| Template | Name | Status |
|---|---|---|
| 5.0  | Simple packing                                  | ✅ |
| 5.2  | Complex packing                                 | ✅ |
| 5.3  | Complex packing + spatial differencing          | ✅ |
| 5.4  | IEEE float                                      | ✅ |
| 5.40 | JPEG2000                                        | ✅ — via libopenjp2 (CGo) |
| 5.41 | PNG                                             | ✅ |
| 5.42 | CCSDS                                           | ✅ — pure Go (port of libaec) |

### Grid Definition Templates (Section 3)
| Template | Name | Status |
|---|---|---|
| 3.0  | Regular lat/lon                                 | ✅ |
| 3.1  | Rotated lat/lon                                 | ✅ |
| 3.10 | Mercator                                        | ✅ |
| 3.20 | Polar stereographic                             | ✅ |
| 3.30 | Lambert conformal                               | ✅ |
| 3.40 | Gaussian — regular and reduced (pl[] table)     | ✅ |
| 3.101 | General unstructured / icosahedral (ICON triangular mesh) | ✅ — coordinates injected via `LoadHorizontalConstants` |

## Install

```sh
go get github.com/pspoerri/go-tiled-eccodes
```

Requires Go 1.21+ and `golang.org/x/sys`. No other dependencies for the
default pure-Go build. To decode JPEG2000 (5.40) messages, see *Optional
packings* below. CCSDS (5.42) is pure Go — no extra setup required.

## Optional packings

JPEG2000 is decoded by linking against a widely-available system library.
It is gated by Go's standard `cgo` build tag. CCSDS (5.42) is a pure-Go
port of libaec and requires no system library.

| Packing | Library | Required for |
|---|---|---|
| 5.40 JPEG2000 | `libopenjp2` (OpenJPEG 2.x) | DWD ICON-Global pre-2024, ECMWF reanalysis archives |

Install the library:

```sh
# macOS
brew install openjpeg

# Debian/Ubuntu
apt install libopenjp2-7-dev

# Fedora/RHEL
dnf install openjpeg2-devel
```

Then build with CGo enabled (the Go default):

```sh
go build ./...                    # CGO_ENABLED defaults to 1; JPEG2000 included
CGO_ENABLED=0 go build ./...      # pure-Go build; 5.40 returns ErrCgoRequired
```

When CGo is off and a JPEG2000 message arrives, `DecodeFloat64` returns
`decode.ErrCgoRequired`. Detect with `errors.Is(err, decode.ErrCgoRequired)`
and fall back to a pure-Go re-encoding pipeline if your deployment requires
a fully-static binary.

### libeccodes (test-only cross-validation)

The `eccodestest` subpackage compiles only when both CGo and the `eccodes`
build tag are enabled. It produces synthetic GRIB2 files via the `writer`
package and re-reads them with ECMWF's reference parser, libeccodes — an
independent confirmation that the writer's output is spec-compliant. The
dependency is opt-in: nothing in the main module links against libeccodes.

```sh
# macOS
brew install eccodes

# Debian/Ubuntu
apt install libeccodes-dev

# Fedora/RHEL
dnf install eccodes-devel

# Run the cross-validation suite
go test -tags eccodes ./eccodestest
```

## Quick start

```go
import (
    "fmt"

    grib "github.com/pspoerri/go-tiled-eccodes"
    "github.com/pspoerri/go-tiled-eccodes/tile"
)

f, _ := grib.Open("icon-d2_t_2m.grib2")
defer f.Close()

m := f.Messages()[0]
h := m.Header()
fmt.Printf("%dx%d points, packing template %d\n", h.Ni, h.Nj, h.DataTemplate)

// Single-point lookup (warm: ~88 ns)
v, _ := m.ValueAt(50.11, 8.68, tile.Bicubic)
fmt.Printf("Frankfurt T2m: %.2f K\n", v)

// 256x256 WGS84 tile (warm: ~2.3 ms on M5 Pro)
dst := make([]float32, 256*256)
m.RenderFloat32(grib.TileRequest{
    Tile:   tile.XYZ{Z: 5, X: 17, Y: 10},
    Width:  256,
    Height: 256,
    Sample: tile.Bicubic,
}, dst)
```

## API at a glance

```go
// Open the file. Indexes messages without decoding their payloads.
func Open(path string) (*File, error)
func FromBytes(data []byte) (*File, error)

func (f *File) Messages() []*Message
func (f *File) Close() error

// Per-message metadata, cheap (no allocations).
func (m *Message) Header() Header
func (m *Message) Grid() (grid.Grid, error)

// Decode the full grid in the message's own storage (scan) order. The first
// call decodes; subsequent calls return a copy of the cached buffer. Pass
// dst to reuse a buffer.
func (m *Message) DecodeFloat32(dst []float32) ([]float32, error)
func (m *Message) DecodeFloat64(dst []float64) ([]float64, error)

// Normalized regular lat/lon access. RegularLatLon resolves the WMO scan bits
// into a grid-def with signed steps (DLat<0 N→S, DLon>0 W→E) anchored at the
// NW grid point; ok=false for non-template-3.0 grids. DecodeNatural* fills dst
// in guaranteed natural order (W→E within rows, rows N→S), pairing 1:1 with
// that grid-def — value[row*Nx+col] is at (Lat0+row*DLat, Lon0+col*DLon).
func (m *Message) RegularLatLon() (RegularLatLon, bool)
func (m *Message) DecodeNaturalFloat32(dst []float32) ([]float32, error)
func (m *Message) DecodeNaturalFloat64(dst []float64) ([]float64, error)

// Single-point lookup at WGS84 (lat, lon).
func (m *Message) ValueAt(lat, lon float64, mode tile.SampleMode) (float64, error)

// Tile renderers. dst length must be req.Width * req.Height. Buffer
// reuse is encouraged.
func (m *Message) RenderFloat32(req TileRequest, dst []float32) error
func (m *Message) RenderFloat64(req TileRequest, dst []float64) error
func (m *Message) RenderInt8 (req TileRequest, q tile.Quantize, dst []int8 ) error
func (m *Message) RenderInt16(req TileRequest, q tile.Quantize, dst []int16) error
func (m *Message) RenderInt32(req TileRequest, q tile.Quantize, dst []int32) error
func (m *Message) RenderInt64(req TileRequest, q tile.Quantize, dst []int64) error
func (m *Message) RenderUint8 (req TileRequest, q tile.Quantize, dst []uint8 ) error
func (m *Message) RenderUint16(req TileRequest, q tile.Quantize, dst []uint16) error
func (m *Message) RenderUint32(req TileRequest, q tile.Quantize, dst []uint32) error
func (m *Message) RenderUint64(req TileRequest, q tile.Quantize, dst []uint64) error

// Region renderers. Sample over an arbitrary WGS84 bbox at Plate-Carrée
// spacing. Useful for GeoJSON data grids, contour-extraction backing
// buffers, or animation tile chunks that don't fit the XYZ tile path.
func (m *Message) RenderRegionFloat32(r Region, dst []float32) error
func (m *Message) RenderRegionFloat64(r Region, dst []float64) error

// Unstructured / icosahedral grids (template 3.101). Coordinates are not
// transmitted with the data and must be loaded from a companion
// horizontal_constants file.
func LoadHorizontalConstants(path string) (*HorizontalConstants, error)
func (f *File) AttachCoordinates(hc *HorizontalConstants) (count int, err error)
func (m *Message) SetGridCoordinates(lats, lons []float64) error
```

## Unstructured (ICON) grids

ICON's triangular icosahedral mesh ships its data on Section 3 template
**3.101**. The geometry of each cell is *not* in the data file — DWD and
MeteoSwiss publish a separate `horizontal_constants_*.grib2` per
resolution that contains the per-cell `clat` / `clon` arrays. Wire them
in once at open time:

```go
hc, _ := grib.LoadHorizontalConstants("horizontal_constants_icon-d2.grib2")
f, _ := grib.Open("icon-d2_t_2m.grib2")
defer f.Close()

if _, err := f.AttachCoordinates(hc); err != nil { … }

// Sample / render works the same as for any other grid.
v, _ := f.Messages()[0].ValueAt(50.11, 8.68, tile.Nearest)
```

`Locate` is backed by a balanced 3-D KD-tree over unit-sphere chord
distance, which sidesteps the antimeridian and pole singularities that
2-D (lat, lon) trees hit. The tree is built once per UUID and shared
across every message that references the same mesh — so a forecast with
100 fields on the same icosahedral grid pays the build cost exactly once.

For regional models on a global icosahedral mesh, set a footprint cap so
queries outside the model's domain return out-of-bounds (and tile pixels
become NaN) rather than picking the nearest cell across the ocean:

```go
g, _ := m.Grid()
g.(*grid.Unstructured).SetMaxDistance(20_000) // 20 km — typical ICON edge buffer
```

## Buffer types

The renderer fills typed destination slices in place: `float32`,
`float64`, and the eight signed/unsigned integer widths. Integer
renderers apply a `Quantize` to map physical values into the integer
range.

```go
type Quantize struct {
    Scale, Offset float64    // out = clamp(round((v - Offset) * Scale), Min, Max)
    Min, Max      float64
    MissingValue  int64      // sentinel cast on output for NaN inputs
}

q := tile.Quantize{Scale: 10, Offset: 273.15, Min: 0, Max: 255, MissingValue: 0}
out := make([]uint8, 256*256)
m.RenderUint8(req, q, out)   // ⌊(K - 273.15) * 10⌋ clamped to [0, 255]
```

## Sampling

| Mode | Use for | Notes |
|---|---|---|
| `tile.Nearest` | discrete fields, fastest path | one source read per pixel |
| `tile.Bicubic` | continuous fields (T, P, RH, U/V) | Catmull-Rom 4×4 stencil; falls back to nearest at NaN borders |
| `tile.Mode`    | categorical fields (precip type, cloud cover class) | histogram of values in a (2·`ModeWindow`+1)² window centered on the pixel; ties broken by lowest value |

## Tile coordinate system

XYZ tiles follow the spherical-Mercator / OSM / "slippy map" convention
used by Mapbox, OpenStreetMap, MapLibre, Leaflet, etc. Tile `(z, x, y)`
covers the bounding box

```
  west  = x        / 2^z * 360 - 180
  east  = (x + 1)  / 2^z * 360 - 180
  north = atan(sinh(π * (1 - 2 *  y      / 2^z))) * 180/π
  south = atan(sinh(π * (1 - 2 * (y + 1) / 2^z))) * 180/π
```

Latitudes are clamped to ~±85.0511° (the Mercator extent). Tile `(0, 0, 0)`
spans the world.

## Concurrency

`*grib.File` and `*grib.Message` are safe for concurrent use after
`Open` returns. Specifically:

- `Header`, `Grid`, `ValueAt`, and the `Render*` methods may be called
  from any number of goroutines concurrently.
- The first concurrent call into a message's decode path races to
  populate the per-message cache via `sync.Once`; subsequent calls
  hit the cached buffer with no synchronisation.
- The mmap is read-only, so concurrent reads are free of data races.

`Close` invalidates the mmap and the cache; ensure no goroutine is
mid-render when you call it.

## Performance

Apple M5 Pro, single core, 1215×746 ICON-D2 t_2m (≈900k grid points,
754k valid):

```
BenchmarkDecodeICONCold              1.5 ms/op    19 allocs (decode 1M points, simple 5.0)
BenchmarkDecodeICONComplex           7.6 ms/op    17 allocs (complex 5.2)
BenchmarkDecodeICONSpdiff            7.7 ms/op    17 allocs (spatial-diff 5.3)
BenchmarkRenderTile256BicubicWarm    2.3 ms/op    6 KB/op   (256² bicubic, cached grid)
BenchmarkValueAtWarm                  88 ns/op    128 B/op  (single-point lookup, cached)
```

The decoded grid is cached on first use; tile renders and `ValueAt`
calls are pure resampling work. Working buffers are pooled by
size bucket via `sync.Pool` so steady-state RSS is dominated by the
mmap and the per-message cache.

## CLI

```sh
# Show every message in a file: parameter, level, time, grid type, packing,
# dimensions.
go run ./cmd/grib-inspect path/to/file.grib2

# Render a single tile to a raw float32 little-endian buffer (W*H values).
go run ./cmd/grib-tile -w 256 -h 256 -sample bicubic \
    path/to/file.grib2 5 17 10 out.bin
```

## Layout

```
grib.go         File / Message / Open / FromBytes
index.go        message indexer (Section 0/8 walker)
header.go       Header summary
decode.go       template-dispatch + bitmap fan-out
render.go       tile renderer + typed buffer adapters

decode/         payload decoders (5.0, 5.2, 5.3, 5.4, 5.40, 5.41, 5.42)
grid/           Section 3 grid types (3.0, 3.1, 3.10, 3.20, 3.30, 3.40, 3.101)
sample/         nearest, bicubic, mode resamplers
section/        zero-copy Section parsers
tile/           XYZ <-> WGS84 mapping + TileRequest

cmd/
  grib-inspect/ list messages and headers
  grib-tile/    render a single tile to disk

internal/
  mmap/         x/sys mmap helpers (linux/darwin/windows)
  bitstream/    GRIB big-endian bit reader (fast paths for 8/16/24/32)
  bufpool/      sync.Pool-backed typed scratch buffers
  bswap/        big-endian + GRIB sign-magnitude readers

testdata/       small public GRIB2 fixtures (ICON-D2, eccodes samples)
```

## Tested with

| Source | Grid | Packing |
|---|---|---|
| DWD ICON-D2 t_2m forecast (`opendata.dwd.de`) | regular lat/lon | simple, complex, spatial-diff |
| ECMWF eccodes `regular_gg_sfc` template | regular Gaussian | simple |
| ECMWF eccodes `reduced_gg_pl_128` template | reduced Gaussian (pl[]) | simple |
| ECMWF eccodes `polar_stereographic_sfc` template | polar stereographic | simple |
| ECMWF eccodes `regular_ll_sfc` template | regular lat/lon (constant field) | simple |

The simple/complex/spatial-diff variants of the same ICON-D2 field were
generated with `grib_set -s packingType=…` and verified to round-trip
within 1e-3 K of each other.

## Status

All seven supported GRIB2 grid types (Section 3 templates 3.0, 3.1,
3.10, 3.20, 3.30, 3.40, 3.101) and seven Section 5 packings (5.0, 5.2,
5.3, 5.4, 5.40, 5.41, 5.42) decode end-to-end. ICON-D2 regular lat/lon
distributions round-trip within 1e-3 K across simple, complex, and
spatial-differencing encodings of the same field.

## License

MIT — see [LICENSE](LICENSE).
