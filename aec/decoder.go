package aec

// decoder holds all per-Decode state. Built by newDecoder; driven by run.
type decoder struct {
	cfg            Config
	idLen          int
	idMax          uint32
	bytesPerSample int
	blockSize      int
	rsi            int // blocks per RSI
	rsiSize        int // samples per RSI
	xmin, xmax     uint32
	pp, signed     bool
	msb            bool

	seTable []int
	rsiBuf  []uint32
	rsip    int // samples buffered in the current RSI

	br      bitReader
	dst     []byte
	outPos  int    // bytes written to dst
	needed  int    // total samples to emit
	emitted int    // samples emitted
	lastOut uint32 // predictor carry
}

// run is implemented in Task 4.
func (d *decoder) run() error { return nil }
