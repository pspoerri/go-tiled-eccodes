//go:build cgo

// JPEG2000 decoder — links against the system libopenjp2. The matching
// nocgo stub returns ErrCgoRequired when CGo is off.
//
// Requires libopenjp2 (OpenJPEG 2.x) at link time. Common install paths:
//   - macOS Homebrew: /opt/homebrew/Cellar/openjpeg/<ver>/{include,lib}
//     (resolved via pkg-config below)
//   - Debian/Ubuntu:  apt install libopenjp2-7-dev
//   - Fedora/RHEL:    dnf install openjpeg2-devel
//
// We rely on pkg-config to find the right headers and the right -lopenjp2
// link line, which keeps the build portable across distros.
package decode

/*
#cgo pkg-config: libopenjp2
#include <stdlib.h>
#include <string.h>
#include <openjpeg.h>

// In-memory stream callbacks for OpenJPEG. OpenJPEG models input as a
// callback-driven stream; we wrap a Go-allocated byte slice with a small
// position tracker and three callbacks so the codec can read/skip/seek
// without ever touching the filesystem.
typedef struct opj_buffer_info {
    unsigned char *buf;
    OPJ_SIZE_T cur;
    OPJ_SIZE_T len;
} opj_buffer_info_t;

static OPJ_SIZE_T j2k_read_from_buffer(void *buffer, OPJ_SIZE_T n, void *user_data) {
    opj_buffer_info_t *info = (opj_buffer_info_t *)user_data;
    OPJ_SIZE_T avail = info->len - info->cur;
    if (avail == 0) return (OPJ_SIZE_T)-1; // signals EOF to OpenJPEG
    if (n > avail) n = avail;
    memcpy(buffer, info->buf + info->cur, n);
    info->cur += n;
    return n;
}

static OPJ_OFF_T j2k_skip_from_buffer(OPJ_OFF_T n, void *user_data) {
    opj_buffer_info_t *info = (opj_buffer_info_t *)user_data;
    OPJ_SIZE_T new_cur = info->cur + (OPJ_SIZE_T)n;
    if (new_cur > info->len) new_cur = info->len;
    OPJ_OFF_T skipped = (OPJ_OFF_T)(new_cur - info->cur);
    info->cur = new_cur;
    return skipped;
}

static OPJ_BOOL j2k_seek_from_buffer(OPJ_OFF_T pos, void *user_data) {
    opj_buffer_info_t *info = (opj_buffer_info_t *)user_data;
    if (pos < 0 || (OPJ_SIZE_T)pos > info->len) return OPJ_FALSE;
    info->cur = (OPJ_SIZE_T)pos;
    return OPJ_TRUE;
}

// Suppress libopenjp2's chatty stderr output. GRIB2 producers occasionally
// emit codestreams with markers OpenJPEG warns about ("incomplete bitstream
// at end-of-tile") even when decoding produces correct samples.
static void j2k_quiet(const char *msg, void *client_data) {
    (void)msg;
    (void)client_data;
}

// j2k_install_handlers wires the silent handlers onto the codec. We do
// this in C rather than passing function pointers across CGo because Go's
// CGo cannot directly reference C function pointers by name.
static void j2k_install_handlers(opj_codec_t *codec) {
    opj_set_info_handler(codec, j2k_quiet, NULL);
    opj_set_warning_handler(codec, j2k_quiet, NULL);
    opj_set_error_handler(codec, j2k_quiet, NULL);
}

// Wire the in-memory buffer + the three callbacks onto a fresh stream.
// Returns the stream and stashes the buffer-info pointer in *info_out so
// the caller can free it after destroying the stream.
static opj_stream_t *j2k_make_stream(unsigned char *buf, OPJ_SIZE_T len,
                                     opj_buffer_info_t **info_out) {
    opj_stream_t *stream = opj_stream_create(len, OPJ_TRUE);
    if (!stream) return NULL;

    opj_buffer_info_t *info = (opj_buffer_info_t *)malloc(sizeof(opj_buffer_info_t));
    if (!info) {
        opj_stream_destroy(stream);
        return NULL;
    }
    info->buf = buf;
    info->cur = 0;
    info->len = len;

    opj_stream_set_user_data(stream, info, NULL);
    opj_stream_set_user_data_length(stream, len);
    opj_stream_set_read_function(stream, j2k_read_from_buffer);
    opj_stream_set_skip_function(stream, j2k_skip_from_buffer);
    opj_stream_set_seek_function(stream, j2k_seek_from_buffer);

    *info_out = info;
    return stream;
}
*/
import "C"

import (
	"fmt"
	"unsafe"
)

// jpeg2000Decode runs the JPEG-2000 codestream in `input` through libopenjp2
// and returns numPoints int32 samples (component 0). Multi-component
// codestreams are extremely rare in GRIB2; we ignore extra components and
// log via the returned error if the count looks wrong.
func jpeg2000Decode(input []byte, numPoints int) ([]int32, error) {
	if len(input) == 0 {
		return nil, fmt.Errorf("decode: empty JPEG-2000 input")
	}

	// libopenjp2's read callback writes into the supplied buffer and
	// expects the source to live at a stable address. We hand it the
	// caller's slice directly — it's a read-only view of the section 7
	// payload, which lives in the mmap.
	var info *C.opj_buffer_info_t
	stream := C.j2k_make_stream(
		(*C.uchar)(unsafe.Pointer(&input[0])),
		C.OPJ_SIZE_T(len(input)),
		&info,
	)
	if stream == nil {
		return nil, fmt.Errorf("decode: opj_stream_create failed")
	}
	defer func() {
		C.opj_stream_destroy(stream)
		if info != nil {
			C.free(unsafe.Pointer(info))
		}
	}()

	codec := C.opj_create_decompress(C.OPJ_CODEC_J2K)
	if codec == nil {
		return nil, fmt.Errorf("decode: opj_create_decompress failed")
	}
	defer C.opj_destroy_codec(codec)

	// Suppress libopenjp2's progress / warning / error chatter — we
	// surface failures through return values, and the warnings from
	// some GRIB2 codestreams are spurious.
	C.j2k_install_handlers(codec)

	var params C.opj_dparameters_t
	C.opj_set_default_decoder_parameters(&params)
	if C.opj_setup_decoder(codec, &params) == 0 {
		return nil, fmt.Errorf("decode: opj_setup_decoder failed")
	}

	var image *C.opj_image_t
	if C.opj_read_header(stream, codec, &image) == 0 {
		return nil, fmt.Errorf("decode: opj_read_header failed")
	}
	defer C.opj_image_destroy(image)

	if C.opj_decode(codec, stream, image) == 0 {
		return nil, fmt.Errorf("decode: opj_decode failed")
	}
	if C.opj_end_decompress(codec, stream) == 0 {
		return nil, fmt.Errorf("decode: opj_end_decompress failed")
	}

	// Pull component 0's samples out. GRIB2 always uses single-component
	// codestreams (pixel = scalar physical value).
	if image.numcomps < 1 {
		return nil, fmt.Errorf("decode: J2K image has no components")
	}
	comp := (*C.opj_image_comp_t)(unsafe.Pointer(image.comps))
	w := int(comp.w)
	h := int(comp.h)
	total := w * h
	if total < numPoints {
		return nil, fmt.Errorf("decode: J2K image has %d samples, GRIB expects %d",
			total, numPoints)
	}
	// comp.data is OPJ_INT32 * — copy into a Go slice. GRIB2 stores each
	// physical value as one (x, y) pair in raster order; we hand the raw
	// int32 view back and the public JPEG2000 wrapper applies the
	// reference + scale.
	src := unsafe.Slice((*int32)(unsafe.Pointer(comp.data)), total)
	out := make([]int32, numPoints)
	copy(out, src[:numPoints])
	return out, nil
}
