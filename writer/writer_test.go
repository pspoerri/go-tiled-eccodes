package writer_test

import (
	"math"
	"testing"
	"time"

	grib "github.com/pspoerri/go-tiled-eccodes"
	gridpkg "github.com/pspoerri/go-tiled-eccodes/grid"
	"github.com/pspoerri/go-tiled-eccodes/writer"
)

// baseField returns a Field skeleton with sensible defaults; tests fill in
// Grid + Values + ParameterNumber + ReferenceTime as needed.
func baseField(g writer.Grid, vals []float64) writer.Field {
	return writer.Field{
		Discipline:              0,
		Centre:                  78, // DWD
		ReferenceTime:           time.Date(2026, 5, 8, 0, 0, 0, 0, time.UTC),
		ProductionStatus:        0,
		ParameterCategory:       0, // Temperature
		ParameterNumber:         0, // Air temperature
		UnitOfTimeRange:         1, // hours
		ForecastTime:            0,
		TypeOfFirstFixedSurface: 103, // height above ground
		ScaledValueFirstSurface: 2,
		Grid:                    g,
		Values:                  vals,
		NumBits:                 16,
	}
}

// linearField returns Ni*Nj test values varying smoothly so packing exercises
// the full range. Values are in natural (W→E, N→S row-major) order.
func linearField(ni, nj int, base float64) []float64 {
	out := make([]float64, ni*nj)
	for j := 0; j < nj; j++ {
		for i := 0; i < ni; i++ {
			out[j*ni+i] = base + float64(j)*0.1 + float64(i)*0.01
		}
	}
	return out
}

// roundTrip encodes and decodes via the public grib package, returning the
// decoded messages and the values for each.
func roundTrip(t *testing.T, data []byte) (*grib.File, []*grib.Message) {
	t.Helper()
	f, err := grib.FromBytes(data)
	if err != nil {
		t.Fatalf("FromBytes: %v", err)
	}
	return f, f.Messages()
}

func assertValuesClose(t *testing.T, got, want []float64, tol float64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("len mismatch: got %d, want %d", len(got), len(want))
	}
	for i, w := range want {
		g := got[i]
		switch {
		case math.IsNaN(w) && math.IsNaN(g):
			continue
		case math.IsNaN(w) != math.IsNaN(g):
			t.Fatalf("vals[%d] = %v, want %v (NaN mismatch)", i, g, w)
		case math.Abs(g-w) > tol:
			t.Fatalf("vals[%d] = %v, want %v (diff %v > %v)", i, g, w, math.Abs(g-w), tol)
		}
	}
}

// --- Mode 1: single timestamp, single variable ---------------------------

func TestSingleField(t *testing.T) {
	g := writer.NewLatLon(8, 5, 51, 6, 0.5, 0.4)
	vals := linearField(8, 5, 273)
	data, err := writer.Single(baseField(g, vals))
	if err != nil {
		t.Fatalf("Single: %v", err)
	}
	f, msgs := roundTrip(t, data)
	defer f.Close()
	if len(msgs) != 1 {
		t.Fatalf("messages = %d, want 1", len(msgs))
	}
	got, err := msgs[0].DecodeFloat64(nil)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	assertValuesClose(t, got, vals, 0.01)
}

// --- Mode 2: multiple timestamps -----------------------------------------

func TestMultipleTimestamps(t *testing.T) {
	g := writer.NewLatLon(6, 4, 50, 8, 0.5, 0.4)
	t0 := time.Date(2026, 5, 8, 0, 0, 0, 0, time.UTC)
	var fields []writer.Field
	wantVals := make([][]float64, 0)
	for h := 0; h < 4; h++ {
		v := linearField(6, 4, 270+float64(h))
		f := baseField(g, v)
		f.ReferenceTime = t0.Add(time.Duration(h) * time.Hour)
		f.ForecastTime = int32(h)
		fields = append(fields, f)
		wantVals = append(wantVals, v)
	}
	data, err := writer.Series(fields)
	if err != nil {
		t.Fatalf("Series: %v", err)
	}
	f, msgs := roundTrip(t, data)
	defer f.Close()
	if len(msgs) != 4 {
		t.Fatalf("messages = %d, want 4", len(msgs))
	}
	for i, m := range msgs {
		h := m.Header()
		want := t0.Add(time.Duration(i) * time.Hour)
		if !h.ReferenceTime.Equal(want) {
			t.Errorf("msg %d ref time = %v, want %v", i, h.ReferenceTime, want)
		}
		if int(h.ForecastTime) != i {
			t.Errorf("msg %d forecast = %d, want %d", i, h.ForecastTime, i)
		}
		got, err := m.DecodeFloat64(nil)
		if err != nil {
			t.Fatalf("decode msg %d: %v", i, err)
		}
		assertValuesClose(t, got, wantVals[i], 0.01)
	}
}

// --- Mode 3: multiple variables (one timestamp, bundled) -----------------

func TestMultipleVariables(t *testing.T) {
	g := writer.NewLatLon(6, 4, 50, 8, 0.5, 0.4)
	tref := time.Date(2026, 5, 8, 0, 0, 0, 0, time.UTC)

	tempVals := linearField(6, 4, 280)
	humVals := linearField(6, 4, 50)
	pressVals := linearField(6, 4, 1000)

	temp := baseField(g, tempVals)
	temp.ReferenceTime = tref
	temp.ParameterCategory = 0
	temp.ParameterNumber = 0

	hum := baseField(g, humVals)
	hum.ReferenceTime = tref
	hum.ParameterCategory = 1 // Moisture
	hum.ParameterNumber = 1   // relative humidity

	press := baseField(g, pressVals)
	press.ReferenceTime = tref
	press.ParameterCategory = 3 // Mass
	press.ParameterNumber = 0   // pressure

	data, err := writer.Bundle([]writer.Field{temp, hum, press})
	if err != nil {
		t.Fatalf("Bundle: %v", err)
	}
	f, msgs := roundTrip(t, data)
	defer f.Close()
	if len(msgs) != 3 {
		t.Fatalf("messages = %d, want 3", len(msgs))
	}

	want := [][]float64{tempVals, humVals, pressVals}
	wantParams := [][2]uint8{{0, 0}, {1, 1}, {3, 0}}
	for i, m := range msgs {
		h := m.Header()
		if [2]uint8{h.ParameterCategory, h.ParameterNumber} != wantParams[i] {
			t.Errorf("msg %d params = (%d,%d), want %v",
				i, h.ParameterCategory, h.ParameterNumber, wantParams[i])
		}
		if !h.ReferenceTime.Equal(tref) {
			t.Errorf("msg %d ref time = %v, want %v", i, h.ReferenceTime, tref)
		}
		got, err := m.DecodeFloat64(nil)
		if err != nil {
			t.Fatalf("decode msg %d: %v", i, err)
		}
		// Different magnitudes need different absolute tolerance.
		tol := 0.01
		if i == 2 {
			tol = 0.1
		}
		assertValuesClose(t, got, want[i], tol)
	}
}

// --- Mode 4: matrix (multiple times × multiple variables) ----------------

func TestMatrix(t *testing.T) {
	g := writer.NewLatLon(5, 3, 50, 8, 0.5, 0.4)
	t0 := time.Date(2026, 5, 8, 0, 0, 0, 0, time.UTC)

	const nT, nV = 3, 2
	var groups [][]writer.Field
	wantParams := []struct{ cat, num uint8 }{{0, 0}, {1, 1}}
	for ti := 0; ti < nT; ti++ {
		var bundle []writer.Field
		for vi := 0; vi < nV; vi++ {
			vals := linearField(5, 3, 270+float64(ti)+float64(vi)*100)
			f := baseField(g, vals)
			f.ReferenceTime = t0.Add(time.Duration(ti) * time.Hour)
			f.ForecastTime = int32(ti)
			f.ParameterCategory = wantParams[vi].cat
			f.ParameterNumber = wantParams[vi].num
			bundle = append(bundle, f)
		}
		groups = append(groups, bundle)
	}

	data, err := writer.EncodeFile(groups)
	if err != nil {
		t.Fatalf("EncodeFile: %v", err)
	}
	f, msgs := roundTrip(t, data)
	defer f.Close()
	if len(msgs) != nT*nV {
		t.Fatalf("messages = %d, want %d", len(msgs), nT*nV)
	}
	idx := 0
	for ti := 0; ti < nT; ti++ {
		for vi := 0; vi < nV; vi++ {
			m := msgs[idx]
			h := m.Header()
			wantTime := t0.Add(time.Duration(ti) * time.Hour)
			if !h.ReferenceTime.Equal(wantTime) {
				t.Errorf("msg %d ref time = %v, want %v", idx, h.ReferenceTime, wantTime)
			}
			if h.ParameterCategory != wantParams[vi].cat || h.ParameterNumber != wantParams[vi].num {
				t.Errorf("msg %d params = (%d,%d), want (%d,%d)",
					idx, h.ParameterCategory, h.ParameterNumber,
					wantParams[vi].cat, wantParams[vi].num)
			}
			idx++
		}
	}
}

// --- "Missing" edge cases not covered by checked-in fixtures -------------

func TestBitmapWithMissing(t *testing.T) {
	g := writer.NewLatLon(4, 3, 51, 6, 1, 1)
	vals := []float64{
		1, 2, math.NaN(), 4,
		5, math.NaN(), 7, 8,
		9, 10, 11, math.NaN(),
	}
	data, err := writer.Single(baseField(g, vals))
	if err != nil {
		t.Fatalf("Single: %v", err)
	}
	f, msgs := roundTrip(t, data)
	defer f.Close()
	got, err := msgs[0].DecodeFloat64(nil)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	assertValuesClose(t, got, vals, 0.01)
}

func TestConstantField(t *testing.T) {
	g := writer.NewLatLon(4, 3, 51, 6, 1, 1)
	vals := make([]float64, 12)
	for i := range vals {
		vals[i] = 273.15
	}
	field := baseField(g, vals)
	field.NumBits = 0 // constant-field fast path
	data, err := writer.Single(field)
	if err != nil {
		t.Fatalf("Single: %v", err)
	}
	f, msgs := roundTrip(t, data)
	defer f.Close()
	got, err := msgs[0].DecodeFloat64(nil)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	for i, v := range got {
		if math.Abs(v-273.15) > 1e-4 {
			t.Errorf("vals[%d] = %v, want 273.15", i, v)
		}
	}
}

func TestNonNaturalScanning(t *testing.T) {
	// j-positive (south→north) storage: the writer reorders Values into the
	// flipped layout, the decoder un-flips, and the round-trip still yields
	// the original natural-order Values.
	g := writer.NewLatLon(4, 3, 52, 6, 1, 1)
	g.Scan = writer.Scan{IPositive: true, JPositive: true, Consecutive: true}
	vals := linearField(4, 3, 273)
	data, err := writer.Single(baseField(g, vals))
	if err != nil {
		t.Fatalf("Single: %v", err)
	}
	f, msgs := roundTrip(t, data)
	defer f.Close()
	got, err := msgs[0].DecodeFloat64(nil)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	// Use ValueAt at the NW corner — the natural-order [0] entry — to verify
	// the renderer routes through the flipped storage correctly.
	v, err := msgs[0].ValueAt(52, 6, 0)
	if err != nil {
		t.Fatalf("ValueAt: %v", err)
	}
	if math.Abs(v-vals[0]) > 0.01 {
		t.Errorf("ValueAt NW = %v, want %v", v, vals[0])
	}
	_ = got
}

// --- Projection coverage --------------------------------------------------

func TestProjectionLatLon_ICONGlobal(t *testing.T) {
	// ICON Global publishes interpolated regular-lat-lon products at various
	// resolutions; mock a tiny 0.5°-step global grid here.
	g := writer.NewLatLon(720, 361, 90, 0, 0.5, 0.5)
	vals := make([]float64, g.NumPoints())
	for i := range vals {
		vals[i] = 280 + float64(i%50)*0.1
	}
	data, err := writer.Single(baseField(g, vals))
	if err != nil {
		t.Fatalf("Single: %v", err)
	}
	f, msgs := roundTrip(t, data)
	defer f.Close()

	h := msgs[0].Header()
	if h.GridTemplate != 0 {
		t.Errorf("GridTemplate = %d, want 0", h.GridTemplate)
	}
	if h.Ni != 720 || h.Nj != 361 {
		t.Errorf("dims = %dx%d, want 720x361", h.Ni, h.Nj)
	}
	got, err := msgs[0].DecodeFloat64(nil)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	assertValuesClose(t, got, vals, 0.01)
}

func TestProjectionRotatedLatLon_ICONCH(t *testing.T) {
	// ICON-CH1 / ICON-CH2 (MeteoSwiss) use a rotated lat/lon grid centred on
	// the Alps. The published rotation places the rotated frame's south pole
	// at roughly (-43°, 10°) so the rotated equator sits over Switzerland.
	const ni, nj = 60, 40
	g := writer.NewRotatedLatLon(
		ni, nj,
		2.0, -2.0, // La1, Lo1 in the rotated frame (NW corner of small domain)
		0.05, 0.05, // ~0.05° step (real ICON-CH1 is 0.01°, smaller for test speed)
		-43.0, 10.0, // rotated south pole geographic coords
	)
	vals := linearField(ni, nj, 270)
	data, err := writer.Single(baseField(g, vals))
	if err != nil {
		t.Fatalf("Single: %v", err)
	}
	f, msgs := roundTrip(t, data)
	defer f.Close()

	gd, err := msgs[0].Grid()
	if err != nil {
		t.Fatalf("grid: %v", err)
	}
	rg, ok := gd.(gridpkg.RotatedLatLon)
	if !ok {
		t.Fatalf("grid type %T, want RotatedLatLon", gd)
	}
	if math.Abs(rg.SouthPoleLat-(-43.0)) > 1e-6 || math.Abs(rg.SouthPoleLon-10.0) > 1e-6 {
		t.Errorf("south pole = (%v, %v), want (-43, 10)", rg.SouthPoleLat, rg.SouthPoleLon)
	}
	got, err := msgs[0].DecodeFloat64(nil)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	assertValuesClose(t, got, vals, 0.01)
}

func TestProjectionMercator(t *testing.T) {
	g := writer.NewMercator(
		20, 15,
		60, -10, 30, 30, // bounds (La1/Lo1 NW, La2/Lo2 SE)
		45,           // LaD
		15000, 15000, // 15 km step at LaD
	)
	vals := linearField(20, 15, 290)
	data, err := writer.Single(baseField(g, vals))
	if err != nil {
		t.Fatalf("Single: %v", err)
	}
	f, msgs := roundTrip(t, data)
	defer f.Close()
	if msgs[0].Header().GridTemplate != 10 {
		t.Errorf("GridTemplate = %d, want 10", msgs[0].Header().GridTemplate)
	}
	got, err := msgs[0].DecodeFloat64(nil)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	assertValuesClose(t, got, vals, 0.01)
}

func TestProjectionPolar(t *testing.T) {
	g := writer.NewPolar(
		25, 25,
		60, -135, // La1, Lo1
		60, 250, // LaD, LoV
		25000, 25000, // dx, dy in metres
		true, // north hemisphere
	)
	vals := linearField(25, 25, 250)
	data, err := writer.Single(baseField(g, vals))
	if err != nil {
		t.Fatalf("Single: %v", err)
	}
	f, msgs := roundTrip(t, data)
	defer f.Close()
	if msgs[0].Header().GridTemplate != 20 {
		t.Errorf("GridTemplate = %d, want 20", msgs[0].Header().GridTemplate)
	}
	got, err := msgs[0].DecodeFloat64(nil)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	assertValuesClose(t, got, vals, 0.01)
}

func TestProjectionLambert(t *testing.T) {
	// HRRR-ish Lambert configuration (North America).
	g := writer.NewLambert(
		30, 25,
		21.138, 237.28, // La1, Lo1
		38.5, 262.5, // LaD, LoV
		3000, 3000, // 3 km
		38.5, 38.5, // tangent cone
	)
	vals := linearField(30, 25, 285)
	data, err := writer.Single(baseField(g, vals))
	if err != nil {
		t.Fatalf("Single: %v", err)
	}
	f, msgs := roundTrip(t, data)
	defer f.Close()
	if msgs[0].Header().GridTemplate != 30 {
		t.Errorf("GridTemplate = %d, want 30", msgs[0].Header().GridTemplate)
	}
	got, err := msgs[0].DecodeFloat64(nil)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	assertValuesClose(t, got, vals, 0.01)
}

func TestProjectionGaussianRegular(t *testing.T) {
	g := writer.NewGaussian(64, 16, 0, 360-360.0/64)
	vals := linearField(g.Ni, 2*g.N, 270)
	data, err := writer.Single(baseField(g, vals))
	if err != nil {
		t.Fatalf("Single: %v", err)
	}
	f, msgs := roundTrip(t, data)
	defer f.Close()
	h := msgs[0].Header()
	if h.GridTemplate != 40 {
		t.Errorf("GridTemplate = %d, want 40", h.GridTemplate)
	}
	got, err := msgs[0].DecodeFloat64(nil)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	assertValuesClose(t, got, vals, 0.02)
}

// --- Mixed-grid file: stress index walker --------------------------------

func TestMixedProjectionsInOneFile(t *testing.T) {
	ll := writer.NewLatLon(6, 4, 50, 0, 1, 1)
	rot := writer.NewRotatedLatLon(6, 4, 0, 0, 1, 1, -43, 10)
	merc := writer.NewMercator(6, 4, 50, 0, 30, 5, 45, 50000, 50000)
	tref := time.Date(2026, 5, 8, 0, 0, 0, 0, time.UTC)

	mk := func(g writer.Grid) writer.Field {
		f := baseField(g, linearField(6, 4, 280))
		f.ReferenceTime = tref
		return f
	}
	data, err := writer.EncodeFile([][]writer.Field{
		{mk(ll)},
		{mk(rot)},
		{mk(merc)},
	})
	if err != nil {
		t.Fatalf("EncodeFile: %v", err)
	}
	f, msgs := roundTrip(t, data)
	defer f.Close()
	if len(msgs) != 3 {
		t.Fatalf("messages = %d, want 3", len(msgs))
	}
	wantTmpl := []uint16{0, 1, 10}
	for i, m := range msgs {
		if got := m.Header().GridTemplate; got != wantTmpl[i] {
			t.Errorf("msg %d template = %d, want %d", i, got, wantTmpl[i])
		}
	}
}

// --- External Grid implementation: prove the interface is usable ---------

// flippedLatLon is a user-side Grid implementation that wraps writer.LatLon
// and inverts i. Demonstrates that the exported Grid interface is enough to
// drive EncodeFile from outside the writer package.
type flippedLatLon struct {
	writer.LatLon
}

func (g flippedLatLon) StorageIndex(i, j int) int {
	// Same arithmetic as a Scan{} with !IPositive — we override here just to
	// prove the dispatch reaches the user implementation.
	if i < 0 || i >= g.Ni || j < 0 || j >= g.Nj {
		return -1
	}
	return j*g.Ni + (g.Ni - 1 - i)
}

func (g flippedLatLon) EncodeTemplate() []byte {
	body := g.LatLon.EncodeTemplate()
	body[57] = (writer.Scan{IPositive: false, Consecutive: true}).Byte()
	return body
}

func TestExternalGridImplementation(t *testing.T) {
	g := flippedLatLon{LatLon: writer.NewLatLon(6, 4, 50, 0, 1, 1)}
	vals := linearField(6, 4, 273)
	data, err := writer.Single(baseField(g, vals))
	if err != nil {
		t.Fatalf("Single: %v", err)
	}
	f, msgs := roundTrip(t, data)
	defer f.Close()
	got, err := msgs[0].DecodeFloat64(nil)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	// ValueAt the NW corner — the natural [0] index — must round-trip even
	// though the values were stored with i flipped.
	v, err := msgs[0].ValueAt(50, 0, 0)
	if err != nil {
		t.Fatalf("ValueAt: %v", err)
	}
	if math.Abs(v-vals[0]) > 0.01 {
		t.Errorf("ValueAt NW = %v, want %v", v, vals[0])
	}
	_ = got
}
