//go:build cgo

// Package-level CCSDS / libaec decoder. This file links against the system
// libaec; the matching nocgo stub returns ErrCgoRequired when CGo is off.
//
// Requires libaec to be installed at link time. Common install paths:
//   - macOS Homebrew: /opt/homebrew/{include,lib}
//   - macOS MacPorts: /opt/local/{include,lib}
//   - Debian/Ubuntu:  /usr/include/libaec.h, /usr/lib/x86_64-linux-gnu/libaec.so
//   - Fedora/RHEL:    /usr/include/libaec.h, /usr/lib64/libaec.so
//
// If libaec lives somewhere unusual, override at build time:
//
//	CGO_CFLAGS="-I/path/to/include" CGO_LDFLAGS="-L/path/to/lib" go build ./...
package decode

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

// ccsdsDecode runs aec_buffer_decode on the input bitstream, writing
// numPoints samples (each `bytesPerSample` wide in host endianness) into
// the destination buffer. flags is the GRIB2 byte 11 mask, which libaec
// consumes directly (the bit assignments line up).
//
// Go's cgo pointer rules require that any Go-allocated memory referenced
// from a struct passed to C be pinned for the duration of the call —
// otherwise the GC could relocate or collect it. We pin both the input
// (read-only view of the section 7 mmap or a Go slice) and the output.
func ccsdsDecode(input, output []byte, bitsPerSample int, blockSize, rsi, flags uint) error {
	if len(input) == 0 {
		return fmt.Errorf("decode: empty CCSDS input")
	}
	var pinner runtime.Pinner
	defer pinner.Unpin()
	pinner.Pin(&input[0])
	if len(output) > 0 {
		pinner.Pin(&output[0])
	}

	var strm C.struct_aec_stream
	strm.next_in = (*C.uchar)(unsafe.Pointer(&input[0]))
	strm.avail_in = C.size_t(len(input))
	if len(output) > 0 {
		strm.next_out = (*C.uchar)(unsafe.Pointer(&output[0]))
		strm.avail_out = C.size_t(len(output))
	}
	strm.bits_per_sample = C.uint(bitsPerSample)
	strm.block_size = C.uint(blockSize)
	strm.rsi = C.uint(rsi)
	strm.flags = C.uint(flags)

	rc := C.aec_buffer_decode(&strm)
	if rc != C.AEC_OK {
		return fmt.Errorf("decode: libaec aec_buffer_decode rc=%d", int(rc))
	}
	if int(strm.total_out) != len(output) {
		return fmt.Errorf("decode: libaec produced %d bytes, expected %d", int(strm.total_out), len(output))
	}
	return nil
}
