//go:build libaec

package aec

// libaec_cgo.go provides CGo bindings to the system libaec library and is
// built only with -tags libaec. It is used by libaec_test.go to generate and
// verify golden test vectors.
//
// Install libaec first (macOS: `brew install libaec`).

/*
#cgo darwin CFLAGS: -I/opt/homebrew/include
#cgo darwin LDFLAGS: -L/opt/homebrew/lib -laec
#cgo linux LDFLAGS: -laec
#include <stdlib.h>
#include <string.h>
#include <libaec.h>
*/
import "C"

import (
	"fmt"
	"runtime"
	"unsafe"
)

// libaecEncode encodes raw sample bytes with libaec, returning the bitstream.
func libaecEncode(raw []byte, cfg Config) ([]byte, error) {
	out := make([]byte, len(raw)+len(raw)/2+4096)

	var pinner runtime.Pinner
	defer pinner.Unpin()

	var strm C.struct_aec_stream
	if len(raw) > 0 {
		pinner.Pin(&raw[0])
		strm.next_in = (*C.uchar)(unsafe.Pointer(&raw[0]))
	}
	pinner.Pin(&out[0])
	strm.avail_in = C.size_t(len(raw))
	strm.next_out = (*C.uchar)(unsafe.Pointer(&out[0]))
	strm.avail_out = C.size_t(len(out))
	strm.bits_per_sample = C.uint(cfg.BitsPerSample)
	strm.block_size = C.uint(cfg.BlockSize)
	strm.rsi = C.uint(cfg.RSI)
	strm.flags = C.uint(cfg.Flags)
	if rc := C.aec_buffer_encode(&strm); rc != C.AEC_OK {
		return nil, fmt.Errorf("aec_buffer_encode rc=%d", int(rc))
	}
	return out[:int(strm.total_out)], nil
}

// libaecDecode decodes with libaec into a buffer of exactly outLen bytes.
func libaecDecode(stream []byte, outLen int, cfg Config) ([]byte, error) {
	out := make([]byte, outLen)

	var pinner runtime.Pinner
	defer pinner.Unpin()

	var strm C.struct_aec_stream
	if len(stream) > 0 {
		pinner.Pin(&stream[0])
		strm.next_in = (*C.uchar)(unsafe.Pointer(&stream[0]))
	}
	pinner.Pin(&out[0])
	strm.avail_in = C.size_t(len(stream))
	strm.next_out = (*C.uchar)(unsafe.Pointer(&out[0]))
	strm.avail_out = C.size_t(outLen)
	strm.bits_per_sample = C.uint(cfg.BitsPerSample)
	strm.block_size = C.uint(cfg.BlockSize)
	strm.rsi = C.uint(cfg.RSI)
	strm.flags = C.uint(cfg.Flags)
	if rc := C.aec_buffer_decode(&strm); rc != C.AEC_OK {
		return nil, fmt.Errorf("aec_buffer_decode rc=%d", int(rc))
	}
	return out, nil
}
