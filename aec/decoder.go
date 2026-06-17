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

// run drives the block loop until enough samples are decoded to fill dst,
// then flushes the trailing partial RSI. Mirrors aec_decode's main loop for
// the whole-buffer (AEC_FLUSH) case.
func (d *decoder) run() error {
	for d.emitted+d.rsip < d.needed {
		if err := d.decodeBlock(); err != nil {
			return err
		}
		if d.rsip >= d.rsiSize {
			d.flush(d.rsiBuf[:d.rsiSize])
			d.rsip = 0
		}
	}
	if d.rsip > 0 {
		d.flush(d.rsiBuf[:d.rsip])
		d.rsip = 0
	}
	return nil
}

// decodeBlock reads one block's option id and dispatches. The first block of
// each RSI (when preprocessing) carries a reference sample.
func (d *decoder) decodeBlock() error {
	ref := 0
	if d.pp && d.rsip == 0 {
		ref = 1
	}
	id, ok := d.br.getBits(d.idLen)
	if !ok {
		return ErrShortInput
	}
	switch {
	case id == 0:
		return d.lowEntropy(ref)
	case id == d.idMax:
		return d.uncomp()
	default:
		return d.split(int(id)-1, ref)
	}
}

// uncomp reads block_size raw bits_per_sample literals. (libaec m_uncomp reads
// the full block regardless of ref; slot 0 of the RSI is implicitly the
// reference and is treated as such at flush time.)
func (d *decoder) uncomp() error {
	for i := 0; i < d.blockSize; i++ {
		v, ok := d.br.getBits(d.cfg.BitsPerSample)
		if !ok {
			return ErrShortInput
		}
		d.rsiBuf[d.rsip] = v
		d.rsip++
	}
	return nil
}

// split decodes a sample-splitting block with split parameter k (= id-1).
// libaec stores all encoded_block_size FS high parts first, then all k-bit
// remainders. When ref==1 the first slot is a raw reference sample.
func (d *decoder) split(k, ref int) error {
	if ref == 1 {
		v, ok := d.br.getBits(d.cfg.BitsPerSample)
		if !ok {
			return ErrShortInput
		}
		d.rsiBuf[d.rsip] = v
		d.rsip++
	}
	ebs := d.blockSize - ref
	base := d.rsip
	for i := 0; i < ebs; i++ {
		fs, ok := d.br.getFS()
		if !ok {
			return ErrShortInput
		}
		d.rsiBuf[base+i] = fs << uint(k)
	}
	if k > 0 {
		for i := 0; i < ebs; i++ {
			rem, ok := d.br.getBits(k)
			if !ok {
				return ErrShortInput
			}
			d.rsiBuf[base+i] += rem
		}
	}
	d.rsip = base + ebs
	return nil
}

// lowEntropy handles id==0: read a 1-bit sub-id, then (if ref) the reference
// sample, then dispatch to second extension (sub-id 1) or zero block (sub-id 0).
// Mirrors m_low_entropy + m_low_entropy_ref.
func (d *decoder) lowEntropy(ref int) error {
	sub, ok := d.br.getBits(1)
	if !ok {
		return ErrShortInput
	}
	if ref == 1 {
		v, ok := d.br.getBits(d.cfg.BitsPerSample)
		if !ok {
			return ErrShortInput
		}
		d.rsiBuf[d.rsip] = v
		d.rsip++
	}
	if sub == 1 {
		return d.secondExtension(ref)
	}
	return d.zeroBlock(ref)
}

// zeroBlock emits a run of zero samples. The FS value gives zero_blocks = fs+1
// with the ROS (remainder-of-segment, value 5) special case. Mirrors m_zero_block.
func (d *decoder) zeroBlock(ref int) error {
	fs, ok := d.br.getFS()
	if !ok {
		return ErrShortInput
	}
	const ros = 5
	zb := int(fs) + 1
	switch {
	case zb == ros:
		b := d.rsip / d.blockSize // completed blocks in this RSI (ref slot already counted)
		zb = min(d.rsi-b, 64-(b%64))
	case zb > ros:
		zb--
	}
	zeroSamples := zb*d.blockSize - ref
	if zeroSamples < 0 || d.rsip+zeroSamples > d.rsiSize {
		return ErrData
	}
	for i := 0; i < zeroSamples; i++ {
		d.rsiBuf[d.rsip] = 0
		d.rsip++
	}
	return nil
}

// secondExtension decodes block_size/2 gamma values into sample pairs via the
// SE inverse table. The loop starts at i=ref so the reference sample (already
// placed by lowEntropy) keeps the pair parity. Mirrors m_se.
func (d *decoder) secondExtension(ref int) error {
	i := ref
	for i < d.blockSize {
		m, ok := d.br.getFS()
		if !ok {
			return ErrShortInput
		}
		if m > seTableSize {
			return ErrData
		}
		d1 := int32(m) - int32(d.seTable[2*m+1])
		if i&1 == 0 {
			d.rsiBuf[d.rsip] = uint32(int32(d.seTable[2*m]) - d1)
			d.rsip++
			i++
		}
		d.rsiBuf[d.rsip] = uint32(d1)
		d.rsip++
		i++
	}
	return nil
}

// flush reverses preprocessing (if enabled) over a full RSI buffer and
// serializes the samples to dst. With whole-buffer decode, flush always starts
// at buf[0], so the reference is buf[0] and the predictor resets each RSI.
// Mirrors libaec's FLUSH macro.
func (d *decoder) flush(buf []uint32) {
	if !d.pp {
		// Cap to the number of samples we still need.
		if rem := d.needed - d.emitted; rem < len(buf) {
			buf = buf[:rem]
		}
		d.writeSamples(buf)
		return
	}

	last := buf[0]
	if d.signed {
		m := uint32(1) << uint(d.cfg.BitsPerSample-1)
		last = (last ^ m) - m // sign-extend the reference
	}
	// We decode into d.rsiBuf in place, reusing the slice for writeSamples.
	// Overwrite buf[0] with the decoded reference so writeSamples can handle
	// it uniformly.
	buf[0] = last
	data := last
	xmax := d.xmax
	n := len(buf)
	if rem := d.needed - d.emitted; rem < n {
		n = rem
	}
	if d.xmin == 0 {
		med := xmax/2 + 1
		for i := 1; i < n; i++ {
			dd := buf[i]
			halfD := dd>>1 + dd&1
			var mask uint32
			if data&med != 0 {
				mask = xmax
			}
			if halfD <= mask^data {
				data += dd>>1 ^ ^(dd&1 - 1)
			} else {
				data = mask ^ dd
			}
			buf[i] = data
		}
	} else {
		for i := 1; i < n; i++ {
			dd := buf[i]
			halfD := dd>>1 + dd&1
			if int32(data) < 0 {
				if halfD <= xmax+data+1 {
					data += dd>>1 ^ ^(dd&1 - 1)
				} else {
					data = dd - xmax - 1
				}
			} else {
				if halfD <= xmax-data {
					data += dd>>1 ^ ^(dd&1 - 1)
				} else {
					data = xmax - dd
				}
			}
			buf[i] = data
		}
	}
	d.lastOut = data
	d.writeSamples(buf[:n])
}

// writeSamples serializes a slice of decoded samples to dst using the
// configured width and endianness. The width+endianness dispatch is hoisted
// once over the whole slice, eliminating a per-sample switch/branch.
// Mirrors put_msb_*/put_lsb_* but batched.
func (d *decoder) writeSamples(buf []uint32) {
	n := len(buf)
	if n == 0 {
		return
	}
	dst := d.dst[d.outPos:]
	switch d.bytesPerSample {
	case 1:
		dst = dst[:n] // BCE: prove slice is large enough, eliminate per-element checks
		for i, v := range buf {
			dst[i] = byte(v)
		}
	case 2:
		dst = dst[:n*2] // BCE hint
		if d.msb {
			for i, v := range buf {
				dst[2*i], dst[2*i+1] = byte(v>>8), byte(v)
			}
		} else {
			for i, v := range buf {
				dst[2*i], dst[2*i+1] = byte(v), byte(v>>8)
			}
		}
	case 3:
		dst = dst[:n*3] // BCE hint
		if d.msb {
			for i, v := range buf {
				dst[3*i], dst[3*i+1], dst[3*i+2] = byte(v>>16), byte(v>>8), byte(v)
			}
		} else {
			for i, v := range buf {
				dst[3*i], dst[3*i+1], dst[3*i+2] = byte(v), byte(v>>8), byte(v>>16)
			}
		}
	case 4:
		dst = dst[:n*4] // BCE hint
		if d.msb {
			for i, v := range buf {
				dst[4*i], dst[4*i+1], dst[4*i+2], dst[4*i+3] = byte(v>>24), byte(v>>16), byte(v>>8), byte(v)
			}
		} else {
			for i, v := range buf {
				dst[4*i], dst[4*i+1], dst[4*i+2], dst[4*i+3] = byte(v), byte(v>>8), byte(v>>16), byte(v>>24)
			}
		}
	}
	d.outPos += n * d.bytesPerSample
	d.emitted += n
}
