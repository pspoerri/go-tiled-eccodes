//go:build !cgo

package writer

// ccsdsEncode is the nocgo stub. Builds without CGo cannot link libaec, so
// requesting PackingCCSDS returns ErrCCSDSNeedsCgo through the public
// Single / Bundle / EncodeFile entry points. To enable, build with
// CGO_ENABLED=1 (the default) and libaec installed.
func ccsdsEncode(input []byte, bitsPerSample int, blockSize, rsi, flags uint) ([]byte, error) {
	return nil, ErrCCSDSNeedsCgo
}
