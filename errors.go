package grib

import "errors"

var (
	ErrNotGRIB             = errors.New("grib: not a GRIB message (missing magic)")
	ErrUnsupportedEdition  = errors.New("grib: unsupported edition (only GRIB2 supported)")
	ErrTruncated           = errors.New("grib: message truncated")
	ErrBadSection          = errors.New("grib: malformed section")
	ErrEndMarker           = errors.New("grib: missing 7777 end marker")
	ErrUnsupportedTemplate = errors.New("grib: unsupported template")
	ErrUnsupportedGrid     = errors.New("grib: unsupported grid definition template")
	ErrUnsupportedPacking  = errors.New("grib: unsupported data representation template")
	ErrShortBuffer         = errors.New("grib: destination buffer too small")
	ErrNoData              = errors.New("grib: message has no decoded data")
	ErrOutOfBounds         = errors.New("grib: coordinate outside grid")
)
