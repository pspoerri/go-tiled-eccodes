//go:build !cgo

package decode

func jpeg2000Decode(input []byte, numPoints int) ([]int32, error) {
	return nil, ErrCgoRequired
}
