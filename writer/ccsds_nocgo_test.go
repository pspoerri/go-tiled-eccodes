//go:build !cgo

package writer_test

import (
	"errors"
	"testing"

	"github.com/pspoerri/go-tiled-eccodes/writer"
)

// TestCCSDSReturnsCgoRequiredWhenDisabled documents the nocgo failure mode:
// encoding CCSDS without CGo returns the explicit ErrCCSDSNeedsCgo sentinel,
// mirroring the decoder's ErrCgoRequired. A pure-Go build still compiles.
func TestCCSDSReturnsCgoRequiredWhenDisabled(t *testing.T) {
	g := writer.NewLatLon(8, 8, 51, 6, 0.5, 0.5)
	vals := linearField(8, 8, 280)
	f := baseField(g, vals)
	f.Packing = writer.PackingCCSDS
	f.NumBits = 16

	_, err := writer.Single(f)
	if !errors.Is(err, writer.ErrCCSDSNeedsCgo) {
		t.Fatalf("Single err = %v, want ErrCCSDSNeedsCgo", err)
	}
}
