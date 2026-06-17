package decode

import "errors"

var ErrBadComplexStream = errors.New("decode: malformed complex-packed stream")

// ErrCgoRequired is returned by the JPEG2000 (5.40) decoder when the binary
// was built without CGo. JPEG2000 links the system libopenjp2; there is no
// pure-Go fallback. Build with CGO_ENABLED=1 (the Go default) and libopenjp2
// installed to decode template 5.40. (CCSDS, 5.42, is pure Go and needs no CGo.)
var ErrCgoRequired = errors.New("decode: this packing requires a CGo build with the corresponding system library installed")
