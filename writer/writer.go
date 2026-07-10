// Package writer produces synthetic GRIB2 byte streams for tests. It encodes
// minimal but spec-compliant messages: simple packing (Section 5 template 0),
// product definition template 4.0, and any of the rectangular grid templates
// (3.0 lat/lon, 3.1 rotated lat/lon, 3.10 Mercator, 3.20 polar stereographic,
// 3.30 Lambert conformal, 3.40 Gaussian — regular only).
//
// Scope: testing. The writer is the inverse of just enough of the decoder to
// round-trip Field values. It is not a full GRIB2 producer.
package writer

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/png"
	"math"
	"time"
)

// PackingType selects the Section 5 / Section 7 encoding. Default (zero
// value) is simple packing (template 5.0).
type PackingType uint8

const (
	PackingSimple PackingType = iota // 5.0
	PackingIEEE                      // 5.4 — IEEE-754 floats verbatim
	PackingPNG                       // 5.41 — PNG-encoded gridpoint values
)

// Field is a single packed data record: identification + product metadata +
// grid + values. Values are in natural scanning order (W→E within rows, rows
// N→S) regardless of which scanning bits the chosen Grid.encode emits — the
// writer applies scanning by reordering values during pack.
//
// NaN entries in Values become missing points (a Section 6 bitmap is added
// automatically and the value is omitted from Section 7).
type Field struct {
	// Identification (Section 1)
	Discipline          uint8
	Centre              uint16
	SubCentre           uint16
	ReferenceTime       time.Time
	ProductionStatus    uint8
	TypeOfProcessedData uint8

	// Product (Section 4 template 4.0)
	ParameterCategory       uint8
	ParameterNumber         uint8
	TypeOfGeneratingProcess uint8
	UnitOfTimeRange         uint8 // 1 = hour, 0 = minute, etc.
	ForecastTime            int32
	TypeOfFirstFixedSurface uint8
	ScaleFactorFirstSurface int8
	ScaledValueFirstSurface uint32

	// Grid (Section 3) — pick any concrete implementation from grids.go.
	Grid Grid

	// Data
	Values  []float64 // length = Grid.NumPoints(), natural scanning order
	NumBits uint8     // bits per value for simple/PNG packing; 0 = constant field

	// Packing selects the encoding for Section 5/7. Default zero value is
	// PackingSimple (template 5.0). When IEEEPrecision is non-zero the
	// writer infers PackingIEEE for backwards compatibility.
	Packing PackingType

	// IEEEPrecision: 1 = float32, 2 = float64. Only consulted when Packing
	// is PackingIEEE. Defaults to 1 (single precision) when unset.
	IEEEPrecision uint8

	// ReuseBitmap, when set on a non-first field within a Bundle, emits
	// Section 6 with indicator 254 ("reuse bitmap previously defined in
	// this message") instead of a fresh copy. The caller is responsible
	// for ensuring an earlier field in the same physical GRIB2 message
	// produced an actual bitmap (i.e. had matching NaN entries). Saves
	// (NumPoints+7)/8 bytes per repeated field.
	ReuseBitmap bool
}

// EncodeFile concatenates one or more physical GRIB2 messages into a single
// byte stream. Each inner slice is the field group for one physical message:
// pass [][]Field{{f}} for a single record, [][]Field{{a},{b},{c}} for three
// independent messages (typical for "same variable at different timestamps"),
// or [][]Field{{a, b, c}} to bundle several fields under shared S0+S1 (the
// natural form for "multiple variables at one timestamp").
//
// All fields in a group share the Section 0 discipline + Section 1 metadata
// of the group's first field. Section 3..7 are emitted per field, so groups
// may mix grids and parameters freely; the indexer handles "sticky" sections
// transparently.
func EncodeFile(groups [][]Field) ([]byte, error) {
	var out bytes.Buffer
	for i, g := range groups {
		if len(g) == 0 {
			return nil, fmt.Errorf("writer: group %d is empty", i)
		}
		msg, err := encodeMessage(g)
		if err != nil {
			return nil, fmt.Errorf("writer: message %d: %w", i, err)
		}
		out.Write(msg)
	}
	return out.Bytes(), nil
}

// Single is shorthand for one physical message containing one field.
func Single(f Field) ([]byte, error) {
	return EncodeFile([][]Field{{f}})
}

// Series produces one physical message per field. Use for "same variable at
// multiple reference times": each field becomes its own message, no shared
// state.
func Series(fields []Field) ([]byte, error) {
	groups := make([][]Field, len(fields))
	for i, f := range fields {
		groups[i] = []Field{f}
	}
	return EncodeFile(groups)
}

// Bundle puts every field into a single physical message (S2..S7 repeated,
// S0+S1 shared). Use for "multiple variables at one reference time".
func Bundle(fields []Field) ([]byte, error) {
	if len(fields) == 0 {
		return nil, errors.New("writer: Bundle requires at least one field")
	}
	return EncodeFile([][]Field{fields})
}

// EncodeMessage builds one physical GRIB2 message containing the supplied
// fields. All fields share the Section 0 / Section 1 metadata of fields[0];
// each field gets its own Section 3..7 tuple. Exposed so external callers
// can compose messages with custom grouping rules and concatenate the
// outputs themselves; the high-level helpers EncodeFile / Series / Bundle
// just wrap this.
func EncodeMessage(fields []Field) ([]byte, error) {
	if len(fields) == 0 {
		return nil, errors.New("writer: EncodeMessage requires at least one field")
	}
	return encodeMessage(fields)
}

func encodeMessage(fields []Field) ([]byte, error) {
	for i, f := range fields {
		if f.Grid == nil {
			return nil, fmt.Errorf("field %d: Grid is nil", i)
		}
		if got, want := len(f.Values), f.Grid.NumPoints(); got != want {
			return nil, fmt.Errorf("field %d: len(Values)=%d, grid expects %d", i, got, want)
		}
	}

	var body bytes.Buffer
	body.Write(encodeSection1(fields[0]))

	for _, f := range fields {
		s3, err := encodeSection3(f.Grid)
		if err != nil {
			return nil, err
		}
		body.Write(s3)
		body.Write(encodeSection4(f))
		s5, s6, s7 := encodeData(f)
		body.Write(s5)
		body.Write(s6)
		body.Write(s7)
	}
	body.WriteString("7777")

	total := 16 + body.Len()
	out := make([]byte, 0, total)
	out = append(out, encodeSection0(fields[0].Discipline, uint64(total))...)
	out = append(out, body.Bytes()...)
	return out, nil
}

func encodeSection0(discipline uint8, totalLen uint64) []byte {
	s := make([]byte, 16)
	copy(s, "GRIB")
	s[6] = discipline
	s[7] = 2 // edition
	binary.BigEndian.PutUint64(s[8:], totalLen)
	return s
}

func encodeSection1(f Field) []byte {
	s := make([]byte, 21)
	binary.BigEndian.PutUint32(s[0:], uint32(len(s)))
	s[4] = 1
	binary.BigEndian.PutUint16(s[5:], f.Centre)
	binary.BigEndian.PutUint16(s[7:], f.SubCentre)
	s[9] = 0  // master tables version
	s[10] = 0 // local tables version
	s[11] = 1 // significance of reference time = start of forecast
	rt := f.ReferenceTime.UTC()
	binary.BigEndian.PutUint16(s[12:], uint16(rt.Year()))
	s[14] = byte(rt.Month())
	s[15] = byte(rt.Day())
	s[16] = byte(rt.Hour())
	s[17] = byte(rt.Minute())
	s[18] = byte(rt.Second())
	s[19] = f.ProductionStatus
	tp := f.TypeOfProcessedData
	if tp == 0 {
		tp = 1 // forecast products
	}
	s[20] = tp
	return s
}

func encodeSection3(g Grid) ([]byte, error) {
	body := g.EncodeTemplate()
	tmplNum := g.TemplateNumber()
	npoints := g.NumPoints()

	s := make([]byte, 14+len(body))
	binary.BigEndian.PutUint32(s[0:], uint32(len(s)))
	s[4] = 3
	s[5] = 0 // source: standard grid definition
	binary.BigEndian.PutUint32(s[6:], uint32(npoints))
	s[10] = 0 // octets for optional list-of-numbers (no reduced grids here)
	s[11] = 0 // interpretation of list of numbers
	binary.BigEndian.PutUint16(s[12:], tmplNum)
	copy(s[14:], body)
	return s, nil
}

func encodeSection4(f Field) []byte {
	// Section 4 prefix: length(4) + section(1) + numCoords(2) + tmplNum(2) = 9
	// Template 4.0 body = 25 bytes
	s := make([]byte, 9+25)
	binary.BigEndian.PutUint32(s[0:], uint32(len(s)))
	s[4] = 4
	binary.BigEndian.PutUint16(s[5:], 0) // num coordinate values after template
	binary.BigEndian.PutUint16(s[7:], 0) // template 4.0

	t := s[9:]
	t[0] = f.ParameterCategory
	t[1] = f.ParameterNumber
	tgp := f.TypeOfGeneratingProcess
	if tgp == 0 {
		tgp = 2 // forecast
	}
	t[2] = tgp
	t[3] = 0                             // background process identifier
	t[4] = 0                             // analysis/forecast generating process
	binary.BigEndian.PutUint16(t[5:], 0) // hours of obs cutoff
	t[7] = 0
	utr := f.UnitOfTimeRange
	if utr == 0 {
		utr = 1 // hours
	}
	t[8] = utr
	putI32SM(t[9:], f.ForecastTime)
	t[13] = f.TypeOfFirstFixedSurface
	putI8SM(t[14:], f.ScaleFactorFirstSurface)
	binary.BigEndian.PutUint32(t[15:], f.ScaledValueFirstSurface)
	t[19] = 255 // type of second fixed surface = missing
	t[20] = 0
	binary.BigEndian.PutUint32(t[21:], 0xffffffff) // missing
	return s
}

// encodeData encodes Sections 5/6/7 from a field's Values, returning each as
// a complete section ready to concatenate. Reorders values into the storage
// order implied by the grid's scanning bits before packing.
func encodeData(f Field) (s5, s6, s7 []byte) {
	stored := reorderForStorage(f.Grid, f.Values)
	bitmap, nset, hasMissing := buildBitmap(stored)

	valid := make([]float64, 0, nset)
	for _, v := range stored {
		if !math.IsNaN(v) {
			valid = append(valid, v)
		}
	}

	switch f.Packing {
	case PackingIEEE:
		s5, s7 = encodeDataIEEE(valid, nset, f.IEEEPrecision)
	case PackingPNG:
		s5, s7 = encodeDataPNG(valid, nset, f.NumBits)
	default:
		s5, s7 = encodeDataSimple(valid, nset, f.NumBits)
	}

	// Section 6
	switch {
	case f.ReuseBitmap:
		// Indicator 254 — reuse a previously materialised bitmap from this
		// physical message. Section body is empty.
		s6 = make([]byte, 6)
		binary.BigEndian.PutUint32(s6[0:], 6)
		s6[4] = 6
		s6[5] = 254
	case hasMissing:
		s6 = make([]byte, 6+len(bitmap))
		binary.BigEndian.PutUint32(s6[0:], uint32(len(s6)))
		s6[4] = 6
		s6[5] = 0 // bitmap present
		copy(s6[6:], bitmap)
	default:
		s6 = make([]byte, 6)
		binary.BigEndian.PutUint32(s6[0:], 6)
		s6[4] = 6
		s6[5] = 255 // no bitmap
	}

	return s5, s6, s7
}

func encodeDataSimple(valid []float64, nset int, requestedBits uint8) (s5, s7 []byte) {
	refValue, binScale, decScale, nbits, packed := pack(valid, requestedBits)

	// Section 5: 11-byte prefix + 10-byte template body.
	s5 = make([]byte, 21)
	binary.BigEndian.PutUint32(s5[0:], uint32(len(s5)))
	s5[4] = 5
	binary.BigEndian.PutUint32(s5[5:], uint32(nset))
	binary.BigEndian.PutUint16(s5[9:], 0) // template 5.0
	t := s5[11:]
	binary.BigEndian.PutUint32(t[0:], math.Float32bits(refValue))
	putI16SM(t[4:], binScale)
	putI16SM(t[6:], decScale)
	t[8] = nbits
	t[9] = 0 // type of original field values: 0 = floating point

	// Section 7
	s7 = make([]byte, 5+len(packed))
	binary.BigEndian.PutUint32(s7[0:], uint32(len(s7)))
	s7[4] = 7
	copy(s7[5:], packed)
	return s5, s7
}

// encodeDataIEEE emits Section 5 template 5.4 (IEEE-754 floating point) plus
// the matching Section 7 payload. Precision: 1 = float32 (default),
// 2 = float64. The values are written verbatim, big-endian.
func encodeDataIEEE(valid []float64, nset int, precision uint8) (s5, s7 []byte) {
	if precision == 0 {
		precision = 1
	}

	// Section 5: 11-byte prefix + 1-byte template body.
	s5 = make([]byte, 12)
	binary.BigEndian.PutUint32(s5[0:], uint32(len(s5)))
	s5[4] = 5
	binary.BigEndian.PutUint32(s5[5:], uint32(nset))
	binary.BigEndian.PutUint16(s5[9:], 4) // template 5.4
	s5[11] = precision

	// Section 7
	var payload []byte
	switch precision {
	case 2:
		payload = make([]byte, nset*8)
		for i, v := range valid {
			binary.BigEndian.PutUint64(payload[i*8:], math.Float64bits(v))
		}
	default:
		payload = make([]byte, nset*4)
		for i, v := range valid {
			binary.BigEndian.PutUint32(payload[i*4:], math.Float32bits(float32(v)))
		}
	}
	s7 = make([]byte, 5+len(payload))
	binary.BigEndian.PutUint32(s7[0:], uint32(len(s7)))
	s7[4] = 7
	copy(s7[5:], payload)
	return s5, s7
}

// encodeDataPNG emits Section 5 template 5.41 plus a PNG-image Section 7.
// nbits selects the channel depth: 8-bit → image.Gray, 16-bit → image.Gray16.
// Values are quantized into [0, 2^nbits-1] with the same R + X*2^E formula
// the simple-packing path uses, so the decoder reads them back identically.
func encodeDataPNG(valid []float64, nset int, requestedBits uint8) (s5, s7 []byte) {
	if requestedBits != 8 && requestedBits != 16 {
		requestedBits = 16
	}
	refValue, binScale, decScale, nbits, _ := pack(valid, requestedBits)

	// Quantize into the integer range that the chosen pixel type can hold.
	pow2E := math.Ldexp(1, int(binScale))
	maxX := uint32(uint64(1)<<uint(nbits) - 1)

	// PNG dimensions: pick width = nset, height = 1 (a thin strip). The
	// decoder doesn't care about layout — it just iterates pixels in the
	// image's natural order, which matches our flat Section 7 layout.
	w, h := nset, 1
	if nset == 0 {
		w = 1
	}

	var img image.Image
	switch nbits {
	case 8:
		gi := image.NewGray(image.Rect(0, 0, w, h))
		for i, v := range valid {
			diff := v - float64(refValue)
			if diff < 0 {
				diff = 0
			}
			x := uint32(math.Round(diff / pow2E))
			if x > maxX {
				x = maxX
			}
			gi.Pix[i] = byte(x)
		}
		img = gi
	default: // 16
		gi := image.NewGray16(image.Rect(0, 0, w, h))
		for i, v := range valid {
			diff := v - float64(refValue)
			if diff < 0 {
				diff = 0
			}
			x := uint32(math.Round(diff / pow2E))
			if x > maxX {
				x = maxX
			}
			gi.Pix[i*2] = byte(x >> 8)
			gi.Pix[i*2+1] = byte(x)
		}
		img = gi
	}

	var pngBuf bytes.Buffer
	_ = png.Encode(&pngBuf, img)
	payload := pngBuf.Bytes()

	// Section 5: 11-byte prefix + 10-byte template body (mirrors 5.0 layout
	// plus the PNG-specific octets).
	s5 = make([]byte, 21)
	binary.BigEndian.PutUint32(s5[0:], uint32(len(s5)))
	s5[4] = 5
	binary.BigEndian.PutUint32(s5[5:], uint32(nset))
	binary.BigEndian.PutUint16(s5[9:], 41) // template 5.41
	t := s5[11:]
	binary.BigEndian.PutUint32(t[0:], math.Float32bits(refValue))
	putI16SM(t[4:], binScale)
	putI16SM(t[6:], decScale)
	t[8] = nbits
	t[9] = 0 // type of original field values: 0 = floating point

	s7 = make([]byte, 5+len(payload))
	binary.BigEndian.PutUint32(s7[0:], uint32(len(s7)))
	s7[4] = 7
	copy(s7[5:], payload)
	return s5, s7
}

// reorderForStorage takes Values in natural (W→E, N→S, row-major) order and
// returns a buffer in the storage order implied by the grid's scanning bits
// — i.e. exactly the order the decoder will produce after un-applying its
// scanning logic. For a natural grid this is a no-op copy.
func reorderForStorage(g Grid, natural []float64) []float64 {
	n := len(natural)
	stored := make([]float64, n)

	// We map natural[idx(i,j)] → stored[Grid.StorageIndex(i,j)]. The decoder
	// reverses this by routing reads through grid.Grid.Index so the user sees
	// natural order.
	ni, nj := g.NaturalSize()
	for j := 0; j < nj; j++ {
		for i := 0; i < ni; i++ {
			off := g.StorageIndex(i, j)
			if off < 0 || off >= n {
				continue
			}
			stored[off] = natural[j*ni+i]
		}
	}
	return stored
}

func buildBitmap(values []float64) (bitmap []byte, nset int, hasMissing bool) {
	bitmap = make([]byte, (len(values)+7)/8)
	for i, v := range values {
		if !math.IsNaN(v) {
			bitmap[i>>3] |= 1 << uint(7-(i&7))
			nset++
		} else {
			hasMissing = true
		}
	}
	if !hasMissing {
		return nil, len(values), false
	}
	return bitmap, nset, true
}

// pack chooses Section 5 template-0 parameters and emits the bit-packed
// payload for the given valid (non-missing) values. Returns nbits=0 when the
// field is constant (or when caller forces it via Field.NumBits=0).
func pack(valid []float64, requestedBits uint8) (refValue float32, binScale, decScale int16, nbits uint8, packed []byte) {
	if len(valid) == 0 {
		return 0, 0, 0, 0, nil
	}

	minV, maxV := valid[0], valid[0]
	for _, v := range valid[1:] {
		if v < minV {
			minV = v
		}
		if v > maxV {
			maxV = v
		}
	}

	refValue = float32(minV)

	// Force refValue ≤ minV to keep (Y - R) non-negative even after the float32
	// rounding step. Decrement the float32 until it lies on the safe side.
	for float64(refValue) > minV && refValue > 0 {
		refValue = math.Float32frombits(math.Float32bits(refValue) - 1)
	}

	if requestedBits == 0 || minV == maxV {
		return refValue, 0, 0, 0, nil
	}

	nbits = requestedBits
	target := float64(uint64(1)<<uint(nbits) - 1)
	span := maxV - float64(refValue)
	if span <= 0 {
		return refValue, 0, 0, 0, nil
	}
	binScale = int16(math.Ceil(math.Log2(span / target)))
	pow2E := math.Ldexp(1, int(binScale))

	bw := newBitWriter(nbits, len(valid))
	maxX := uint32(target)
	for _, v := range valid {
		diff := v - float64(refValue)
		if diff < 0 {
			diff = 0
		}
		x := uint32(math.Round(diff / pow2E))
		if x > maxX {
			x = maxX
		}
		bw.write(x)
	}
	return refValue, binScale, 0, nbits, bw.finish()
}

type bitWriter struct {
	nbits uint8
	buf   []byte
	bit   int
}

func newBitWriter(nbits uint8, n int) *bitWriter {
	nbytes := (int(nbits)*n + 7) / 8
	return &bitWriter{nbits: nbits, buf: make([]byte, nbytes)}
}

func (w *bitWriter) write(v uint32) {
	for i := int(w.nbits) - 1; i >= 0; i-- {
		bit := byte((v >> uint(i)) & 1)
		if bit != 0 {
			w.buf[w.bit>>3] |= 1 << uint(7-(w.bit&7))
		}
		w.bit++
	}
}

func (w *bitWriter) finish() []byte { return w.buf }

func putI16SM(b []byte, v int16) {
	if v < 0 {
		binary.BigEndian.PutUint16(b, 0x8000|uint16(-v))
	} else {
		binary.BigEndian.PutUint16(b, uint16(v))
	}
}

func putI32SM(b []byte, v int32) {
	if v < 0 {
		binary.BigEndian.PutUint32(b, 0x80000000|uint32(-v))
	} else {
		binary.BigEndian.PutUint32(b, uint32(v))
	}
}

func putI8SM(b []byte, v int8) {
	if v < 0 {
		b[0] = byte(-v) | 0x80
		return
	}
	b[0] = byte(v)
}
