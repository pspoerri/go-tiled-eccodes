//go:build !cgo

package decode

// ccsdsDecode is the nocgo stub. Builds without CGo cannot link libaec, so
// any CCSDS-encoded message returns ErrCgoRequired through the public
// CCSDS function. To enable, build with CGO_ENABLED=1 (the default) and
// libaec installed.
func ccsdsDecode(input, output []byte, bitsPerSample int, blockSize, rsi, flags uint) error {
	return ErrCgoRequired
}
