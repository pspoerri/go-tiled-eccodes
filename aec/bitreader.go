package aec

// bitReader is replaced with a real implementation in Task 2.
type bitReader struct {
	src []byte
	pos int
	acc uint64
	cnt int
}
