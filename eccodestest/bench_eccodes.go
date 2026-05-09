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

// DecodeValuesEccodes decodes only the values array of every message in path
// using libeccodes. This is the closest libeccodes equivalent of the pure-Go
// Message.DecodeFloat64(nil) call exercised in ../bench_test.go.
func DecodeValuesEccodes(path string) error {
	mode := C.CString("r")
	defer C.free(unsafe.Pointer(mode))
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))

	fp := C.fopen(cpath, mode)
	if fp == nil {
		return fmt.Errorf("eccodes: fopen %s failed", path)
	}
	defer C.fclose(fp)

	ck := C.CString("values")
	defer C.free(unsafe.Pointer(ck))

	for {
		var cerr C.int
		h := C.codes_handle_new_from_file(nil, fp, C.PRODUCT_GRIB, &cerr)
		if h == nil {
			if cerr != 0 {
				return fmt.Errorf("eccodes: codes_handle_new_from_file err=%d", int(cerr))
			}
			return nil
		}
		var n C.size_t
		if rc := C.codes_get_size(h, ck, &n); rc != 0 {
			C.codes_handle_delete(h)
			return errors.New("eccodes: codes_get_size(values) failed")
		}
		buf := make([]float64, n)
		sz := n
		rc := C.codes_get_double_array(h, ck, (*C.double)(unsafe.Pointer(&buf[0])), &sz)
		C.codes_handle_delete(h)
		if rc != 0 {
			return errors.New("eccodes: codes_get_double_array(values) failed")
		}
		_ = buf
	}
}
