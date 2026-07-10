package grid

import (
	"encoding/binary"
	"testing"
)

func TestStretchedGaussianIdentity(t *testing.T) {
	base := gaussianTemplateForTest(8, 4, 4, 60, 0, -60, 315, 45, 0)
	template := make([]byte, 70)
	copy(template, base)
	putTestAngle(template[58:], 90)
	putTestAngle(template[62:], 0)
	binary.BigEndian.PutUint32(template[66:], 1_000_000)

	plain := ParseGaussian(base, nil, 0)
	stretched := ParseStretchedGaussian(template, nil, 0)
	for _, point := range [][2]float64{{45, 10}, {0, 180}, {-45, 300}} {
		pi, pj, pok := plain.Locate(point[0], point[1])
		si, sj, sok := stretched.Locate(point[0], point[1])
		if pok != sok || abs(pi-si) > 1e-9 || abs(pj-sj) > 1e-9 {
			t.Errorf("Locate(%v) plain=(%v,%v,%v), stretched=(%v,%v,%v)", point, pi, pj, pok, si, sj, sok)
		}
	}
}

func TestStretchedRotatedGaussianMetadata(t *testing.T) {
	base := gaussianTemplateForTest(8, 4, 4, 60, 0, -60, 315, 45, 0)
	template := make([]byte, 82)
	copy(template, base)
	putTestAngle(template[58:], -30)
	binary.BigEndian.PutUint32(template[62:], 10_000_000)
	binary.BigEndian.PutUint32(template[66:], 0)
	putTestAngle(template[70:], 40)
	putTestAngle(template[74:], -20)
	binary.BigEndian.PutUint32(template[78:], 2_500_000)

	g := ParseStretchedRotatedGaussian(template, nil, 0)
	if g.StretchPoleLat != 40 || g.StretchPoleLon != -20 || g.StretchFactor != 2.5 {
		t.Fatalf("stretch metadata = (%v,%v,%v)", g.StretchPoleLat, g.StretchPoleLon, g.StretchFactor)
	}
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
