//go:build cgo

// CCSDS / libaec encoder for the writer. Mirrors decode/ccsds_cgo.go: it links
// the system libaec and feeds aec_buffer_encode. The matching nocgo stub
// returns ErrCCSDSNeedsCgo when CGo is off.
//
// Requires libaec at link time. Common install paths:
//   - macOS Homebrew: /opt/homebrew/{include,lib}
//   - Debian/Ubuntu:  /usr/include/libaec.h, /usr/lib/x86_64-linux-gnu/libaec.so
//   - Fedora/RHEL:    /usr/include/libaec.h, /usr/lib64/libaec.so
//
// Override an unusual location at build time:
//
//	CGO_CFLAGS="-I/path/to/include" CGO_LDFLAGS="-L/path/to/lib" go build ./...
package writer

/*
#cgo darwin CFLAGS: -I/opt/homebrew/include
#cgo darwin LDFLAGS: -L/opt/homebrew/lib -laec
#cgo linux LDFLAGS: -laec

#include <stdlib.h>
#include <libaec.h>
*/
import "C"

import (
	"fmt"
	"runtime"
	"unsafe"
)

// ccsdsEncode runs aec_buffer_encode over the fixed-width sample stream and
// returns the compressed CCSDS bitstream. bitsPerSample / blockSize / rsi /
// flags must match what gets written into Section 5 so the decoder can read it
// back.
//
// The output buffer is sized with generous headroom: AEC's worst case
// (incompressible data stored near-verbatim) expands only marginally over the
// input, so input + 1/8 + 1 KiB is always sufficient. We verify the encoder
// did not run out of room before trimming to the produced length.
func ccsdsEncode(input []byte, bitsPerSample int, blockSize, rsi, flags uint) ([]byte, error) {
	if len(input) == 0 {
		return nil, fmt.Errorf("writer: empty CCSDS input")
	}
	output := make([]byte, len(input)+len(input)/8+1024)

	var pinner runtime.Pinner
	defer pinner.Unpin()
	pinner.Pin(&input[0])
	pinner.Pin(&output[0])

	var strm C.struct_aec_stream
	strm.next_in = (*C.uchar)(unsafe.Pointer(&input[0]))
	strm.avail_in = C.size_t(len(input))
	strm.next_out = (*C.uchar)(unsafe.Pointer(&output[0]))
	strm.avail_out = C.size_t(len(output))
	strm.bits_per_sample = C.uint(bitsPerSample)
	strm.block_size = C.uint(blockSize)
	strm.rsi = C.uint(rsi)
	strm.flags = C.uint(flags)

	rc := C.aec_buffer_encode(&strm)
	if rc != C.AEC_OK {
		return nil, fmt.Errorf("writer: libaec aec_buffer_encode rc=%d", int(rc))
	}
	n := int(strm.total_out)
	if n >= len(output) {
		return nil, fmt.Errorf("writer: libaec output overflowed (%d ≥ %d)", n, len(output))
	}
	out := make([]byte, n)
	copy(out, output[:n])
	return out, nil
}
