package decode

import (
	"encoding/binary"
	"testing"
)

func validComplexTemplateForTest() []byte {
	t := make([]byte, 36)
	t[8] = 8
	binary.BigEndian.PutUint32(t[20:], 1)
	t[24] = 8
	t[25] = 0
	binary.BigEndian.PutUint32(t[26:], 1)
	t[30] = 1
	binary.BigEndian.PutUint32(t[31:], 1)
	t[35] = 0
	return t
}

func TestComplexRejectsMalformedStreams(t *testing.T) {
	valid := validComplexTemplateForTest()
	got, err := Complex(valid, []byte{2, 3}, 1, nil)
	if err != nil || len(got) != 1 || got[0] != 5 {
		t.Fatalf("valid Complex = %v err=%v, want [5]", got, err)
	}

	for length := 0; length < 36; length++ {
		if _, err := Complex(valid[:length], nil, 1, nil); err == nil {
			t.Errorf("template length %d returned nil error", length)
		}
	}
	if _, err := Complex(valid, []byte{2}, 1, nil); err == nil {
		t.Error("truncated group-value stream returned nil error")
	}

	tooManyGroups := append([]byte(nil), valid...)
	binary.BigEndian.PutUint32(tooManyGroups[20:], 2)
	if _, err := Complex(tooManyGroups, []byte{0, 0, 0}, 1, nil); err == nil {
		t.Error("more groups than values returned nil error")
	}

	badWidth := append([]byte(nil), valid...)
	badWidth[24] = 33
	if _, err := Complex(badWidth, []byte{0, 0}, 1, nil); err == nil {
		t.Error("group width > 32 returned nil error")
	}
}

func TestSpatialDifferencingRejectsMalformedDescriptors(t *testing.T) {
	template := append(validComplexTemplateForTest(), 1, 2)
	if _, err := ComplexSpatialDifferencing(template, []byte{0, 1}, 1, nil); err == nil {
		t.Error("truncated spatial descriptors returned nil error")
	}
	if _, err := ComplexSpatialDifferencing(template, []byte{0, 1, 0, 0}, 0, nil); err == nil {
		t.Error("order greater than value count returned nil error")
	}
	for length := 0; length < 38; length++ {
		if _, err := ComplexSpatialDifferencing(template[:length], nil, 1, nil); err == nil {
			t.Errorf("spatial template length %d returned nil error", length)
		}
	}
}
