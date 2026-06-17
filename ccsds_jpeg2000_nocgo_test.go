//go:build !cgo

package grib_test

import (
	"errors"
	"testing"

	grib "github.com/pspoerri/go-tiled-eccodes"
	"github.com/pspoerri/go-tiled-eccodes/decode"
)

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
