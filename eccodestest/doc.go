// Package eccodestest cross-validates the writer package against ECMWF's
// libeccodes — the reference GRIB2 parser. The tests build only when the
// "eccodes" build tag is supplied, so the dependency on libeccodes is
// strictly opt-in:
//
//	go test -tags eccodes ./eccodestest
//
// It needs the libeccodes development headers + library on the system
// pkg-config path. See README.md for installation instructions.
package eccodestest
