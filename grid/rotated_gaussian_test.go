package grid

import (
	"encoding/binary"
	"math"
	"testing"
)

func TestRotatedGaussianUsesIEEEAngle(t *testing.T) {
	base := gaussianTemplateForTest(4, 4, 2, 87, 0, -87, 270, 90, 0)
	template := make([]byte, 70)
	copy(template, base)
	putTestAngle(template[58:], -30)
	binary.BigEndian.PutUint32(template[62:], 10_000_000)
	binary.BigEndian.PutUint32(template[66:], math.Float32bits(12.5))

	g := ParseRotatedGaussian(template, nil, 0)
	if g.SouthPoleLat != -30 || g.SouthPoleLon != 10 {
		t.Fatalf("south pole = (%v,%v), want (-30,10)", g.SouthPoleLat, g.SouthPoleLon)
	}
	if g.Angle != 12.5 {
		t.Fatalf("angle = %v, want 12.5", g.Angle)
	}
}
