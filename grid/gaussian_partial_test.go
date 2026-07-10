package grid

import (
	"encoding/binary"
	"math"
	"testing"
)

func gaussianTemplateForTest(ni uint32, nj, n int, la1, lo1, la2, lo2, di float64, scan byte) []byte {
	t := make([]byte, 58)
	t[0] = 6
	binary.BigEndian.PutUint32(t[16:], ni)
	binary.BigEndian.PutUint32(t[20:], uint32(nj))
	binary.BigEndian.PutUint32(t[28:], 0xffffffff)
	putTestAngle(t[32:], la1)
	putTestAngle(t[36:], lo1)
	putTestAngle(t[41:], la2)
	putTestAngle(t[45:], lo2)
	if di > 0 {
		binary.BigEndian.PutUint32(t[49:], uint32(math.Round(di*1e6)))
	} else {
		binary.BigEndian.PutUint32(t[49:], 0xffffffff)
	}
	binary.BigEndian.PutUint32(t[53:], uint32(n))
	t[57] = scan
	return t
}

func putTestAngle(dst []byte, value float64) {
	magnitude := uint32(math.Round(math.Abs(value) * 1e6))
	if value < 0 {
		magnitude |= 0x80000000
	}
	binary.BigEndian.PutUint32(dst, magnitude)
}

func TestPartialRegularGaussian(t *testing.T) {
	full := gaussianLatitudes(4)
	template := gaussianTemplateForTest(8, 4, 4, full[2], 10, full[5], 40, 10, 0)
	g := ParseGaussian(template, nil, 0)
	ni, nj := g.Size()
	if ni != 8 || nj != 4 {
		t.Fatalf("Size = %dx%d, want 8x4", ni, nj)
	}
	for i := 0; i < 4; i++ {
		if math.Abs(g.Lats[i]-full[i+2]) > 1e-12 {
			t.Errorf("latitude[%d] = %v, want %v", i, g.Lats[i], full[i+2])
		}
	}
	if _, _, ok := g.Locate(full[1], 20); ok {
		t.Error("latitude outside regional subset was accepted")
	}
	fi, fj, ok := g.Locate(full[3], 30)
	if !ok || math.Abs(fi-2) > 1e-9 || math.Abs(fj-1) > 1e-6 {
		t.Errorf("Locate = (%v,%v,%v), want (2,1,true)", fi, fj, ok)
	}
	if got := g.Index(7, 3); got != 31 {
		t.Errorf("Index(7,3) = %d, want 31", got)
	}
}

func TestReducedGaussianJPositiveRows(t *testing.T) {
	full := gaussianLatitudes(4)
	template := gaussianTemplateForTest(0xffffffff, 4, 4, full[5], 0, full[2], 359, 0, 0x40)
	storagePL := []byte{2, 4, 6, 8}
	g := ParseGaussian(template, storagePL, 1)
	wantNatural := []int{8, 6, 4, 2}
	for i := range wantNatural {
		if g.PL[i] != wantNatural[i] {
			t.Fatalf("PL = %v, want %v", g.PL, wantNatural)
		}
	}
	if got := g.Index(0, 0); got != 12 {
		t.Errorf("natural north-west Index = %d, want storage row 3 offset 12", got)
	}
	if got := g.Index(7, 0); got != 19 {
		t.Errorf("natural north-east Index = %d, want 19", got)
	}
}

func TestParseEarthShapes(t *testing.T) {
	sphere := make([]byte, 16)
	sphere[0], sphere[1] = 1, 0
	binary.BigEndian.PutUint32(sphere[2:], 7000000)
	if got := ParseEarth(sphere).EffectiveRadius(); got != 7000000 {
		t.Errorf("producer sphere radius = %v, want 7000000", got)
	}

	ellipsoid := make([]byte, 16)
	ellipsoid[0], ellipsoid[6], ellipsoid[11] = 3, 0, 0
	binary.BigEndian.PutUint32(ellipsoid[7:], 6378)
	binary.BigEndian.PutUint32(ellipsoid[12:], 6357)
	e := ParseEarth(ellipsoid)
	if e.MajorAxis != 6378000 || e.MinorAxis != 6357000 {
		t.Errorf("producer ellipsoid = %+v", e)
	}
	if r := e.EffectiveRadius(); r <= e.MinorAxis || r >= e.MajorAxis {
		t.Errorf("authalic radius %v outside axes", r)
	}

	mercatorTemplate := make([]byte, 58)
	copy(mercatorTemplate, sphere)
	g := ParseMercator(mercatorTemplate)
	if g.Earth.Radius != 7000000 {
		t.Errorf("Mercator Earth = %+v", g.Earth)
	}
}
