//go:build !cgo

package grib_test

import (
	"errors"
	"testing"

	grib "github.com/pspoerri/go-tiled-eccodes"
	"github.com/pspoerri/go-tiled-eccodes/decode"
)

// TestCCSDSReturnsCgoRequiredWhenDisabled documents the nocgo failure mode:
// decoding a CCSDS-packed message without CGo returns the explicit
// ErrCgoRequired sentinel so callers can fall back gracefully.
func TestCCSDSReturnsCgoRequiredWhenDisabled(t *testing.T) {
	f, err := grib.Open(loadTestdata(t, "icon-d2_t_2m_ccsds.grib2"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()
	_, err = f.Messages()[0].DecodeFloat64(nil)
	if !errors.Is(err, decode.ErrCgoRequired) {
		t.Fatalf("decode err = %v, want ErrCgoRequired", err)
	}
}

func TestJPEG2000ReturnsCgoRequiredWhenDisabled(t *testing.T) {
	f, err := grib.Open(loadTestdata(t, "icon-d2_t_2m_jpeg.grib2"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()
	_, err = f.Messages()[0].DecodeFloat64(nil)
	if !errors.Is(err, decode.ErrCgoRequired) {
		t.Fatalf("decode err = %v, want ErrCgoRequired", err)
	}
}
