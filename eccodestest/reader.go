//go:build eccodes && cgo

package eccodestest

/*
#cgo pkg-config: eccodes
#include <stdio.h>
#include <stdlib.h>
#include <eccodes.h>
*/
import "C"

import (
	"errors"
	"fmt"
	"unsafe"
)

// EccodesMessage carries the per-message values pulled from libeccodes for
// comparison against writer-produced GRIB2. We deliberately read every
// field via the eccodes key API so the cross-check is independent of our
// own parser.
type EccodesMessage struct {
	Edition       int64
	Discipline    int64
	Centre        int64
	Year          int64
	Month, Day    int64
	Hour, Minute  int64
	GridDefNumber int64
	Ni, Nj        int64
	ParamCategory int64
	ParamNumber   int64
	ForecastTime  int64
	NumValues     int64
	BitsPerValue  int64
	Values        []float64
}

// ReadEccodes opens path with libeccodes and returns one EccodesMessage per
// GRIB2 message in the file.
func ReadEccodes(path string) ([]EccodesMessage, error) {
	mode := C.CString("r")
	defer C.free(unsafe.Pointer(mode))
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))

	fp := C.fopen(cpath, mode)
	if fp == nil {
		return nil, fmt.Errorf("eccodes: fopen %s failed", path)
	}
	defer C.fclose(fp)

	var out []EccodesMessage
	for {
		var cerr C.int
		h := C.codes_handle_new_from_file(nil, fp, C.PRODUCT_GRIB, &cerr)
		if h == nil {
			if cerr != 0 {
				return nil, fmt.Errorf("eccodes: codes_handle_new_from_file err=%d", int(cerr))
			}
			break
		}

		msg, err := readMessage(h)
		C.codes_handle_delete(h)
		if err != nil {
			return nil, err
		}
		out = append(out, msg)
	}
	return out, nil
}

func readMessage(h *C.codes_handle) (EccodesMessage, error) {
	var m EccodesMessage
	keys := []struct {
		name string
		dst  *int64
		req  bool
	}{
		{"edition", &m.Edition, true},
		{"discipline", &m.Discipline, true},
		{"centre", &m.Centre, true},
		{"year", &m.Year, true},
		{"month", &m.Month, true},
		{"day", &m.Day, true},
		{"hour", &m.Hour, true},
		{"minute", &m.Minute, true},
		{"gridDefinitionTemplateNumber", &m.GridDefNumber, true},
		{"Ni", &m.Ni, false},
		{"Nj", &m.Nj, false},
		{"parameterCategory", &m.ParamCategory, true},
		{"parameterNumber", &m.ParamNumber, true},
		{"forecastTime", &m.ForecastTime, false},
		{"numberOfDataPoints", &m.NumValues, true},
		{"bitsPerValue", &m.BitsPerValue, false},
	}
	for _, k := range keys {
		v, ok := getLong(h, k.name)
		if !ok && k.req {
			return m, fmt.Errorf("eccodes: required key %q missing", k.name)
		}
		*k.dst = v
	}

	if m.NumValues > 0 {
		var sz C.size_t = C.size_t(m.NumValues)
		vals := make([]float64, sz)
		ck := C.CString("values")
		rc := C.codes_get_double_array(h, ck, (*C.double)(unsafe.Pointer(&vals[0])), &sz)
		C.free(unsafe.Pointer(ck))
		if rc != 0 {
			return m, errors.New("eccodes: codes_get_double_array failed")
		}
		m.Values = vals[:int(sz)]
	}
	return m, nil
}

func getLong(h *C.codes_handle, key string) (int64, bool) {
	ck := C.CString(key)
	defer C.free(unsafe.Pointer(ck))
	var v C.long
	if rc := C.codes_get_long(h, ck, &v); rc != 0 {
		return 0, false
	}
	return int64(v), true
}
