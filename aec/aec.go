// Package aec is a pure-Go decoder for CCSDS 121.0-B-3 adaptive entropy
// coding — the algorithm implemented by libaec and used by GRIB2 Data
// Representation Template 5.42. It is a faithful port of libaec v1.1.7's
// aec_buffer_decode and produces byte-identical output.
//
// Only decoding of a complete buffer is supported (the equivalent of
// libaec's aec_buffer_decode); there is no encoder and no streaming API.
package aec

import "errors"

// Flags mirrors libaec's sample-data description flags. The bit values are
// identical to <libaec.h> so a GRIB2 "CCSDS compression options" byte maps
// straight onto a Flags value.
type Flags uint

const (
	DataSigned      Flags = 1 << iota // samples are signed (two's complement)
	Data3Byte                         // 24-bit samples stored in 3 bytes
	DataMSB                           // output bytes most-significant-first (big-endian)
	DataPreprocess                    // preprocessor (predictor) was applied
	RestrictedCodes                   // restricted set of code options
	PadRSI                            // each RSI padded to a byte boundary
)

// Config describes the bitstream being decoded. The values come from the
// GRIB2 template (or the encoder that produced the stream).
type Config struct {
	BitsPerSample int // 1..32
	BlockSize     int // 8, 16, 32, 64 (must be even, 2..256)
	RSI           int // reference sample interval, in blocks (1..4096)
	Flags         Flags
}

// Exported errors. They wrap the libaec failure modes.
var (
	ErrConfig      = errors.New("aec: invalid configuration")
	ErrData        = errors.New("aec: malformed bitstream")
	ErrShortInput  = errors.New("aec: input ended before all samples were decoded")
	ErrShortOutput = errors.New("aec: dst too small for decoded samples")
)

// Decode decodes the AEC bitstream src into dst, writing BitsPerSample-wide
// samples in the byte layout libaec produces: storage width 1/2/3/4 bytes
// (per BitsPerSample and Data3Byte), big-endian iff DataMSB else
// little-endian. It returns the number of bytes written. dst must be large
// enough for all samples it can decode from src; Decode never grows dst.
func Decode(dst, src []byte, cfg Config) (int, error) {
	d, err := newDecoder(dst, src, cfg)
	if err != nil {
		return 0, err
	}
	if err := d.run(); err != nil {
		return d.outPos, err
	}
	return d.outPos, nil
}

// newDecoder validates cfg and builds a decoder with all derived parameters
// set. It mirrors libaec's aec_decode_init.
func newDecoder(dst, src []byte, cfg Config) (*decoder, error) {
	bps := cfg.BitsPerSample
	if bps < 1 || bps > 32 ||
		cfg.RSI < 1 || cfg.RSI > 4096 ||
		cfg.BlockSize < 2 || cfg.BlockSize > 256 || cfg.BlockSize&1 != 0 {
		return nil, ErrConfig
	}

	d := &decoder{
		cfg:       cfg,
		blockSize: cfg.BlockSize,
		rsi:       cfg.RSI,
		rsiSize:   cfg.RSI * cfg.BlockSize,
		pp:        cfg.Flags&DataPreprocess != 0,
		signed:    cfg.Flags&DataSigned != 0,
		msb:       cfg.Flags&DataMSB != 0,
	}

	switch {
	case bps > 16:
		d.idLen = 5
		if bps <= 24 && cfg.Flags&Data3Byte != 0 {
			d.bytesPerSample = 3
		} else {
			d.bytesPerSample = 4
		}
	case bps > 8:
		d.idLen = 4
		d.bytesPerSample = 2
	default: // bps 1..8
		if cfg.Flags&RestrictedCodes != 0 {
			if bps > 4 {
				return nil, ErrConfig // libaec rejects RESTRICTED with bps 5..8
			}
			if bps <= 2 {
				d.idLen = 1
			} else {
				d.idLen = 2
			}
		} else {
			d.idLen = 3
		}
		d.bytesPerSample = 1
	}
	d.idMax = uint32(1)<<uint(d.idLen) - 1

	// Sample range. unsignedMax = 2^bps - 1 (all-ones in bps bits).
	unsignedMax := ^uint32(0) >> uint(32-bps)
	if d.signed {
		d.xmax = unsignedMax >> 1 // 2^(bps-1) - 1
		d.xmin = ^d.xmax
	} else {
		d.xmin = 0
		d.xmax = unsignedMax
	}

	d.seTable = buildSETable()
	d.rsiBuf = make([]uint32, d.rsiSize)
	d.br = bitReader{src: src}
	d.dst = dst
	d.needed = len(dst) / d.bytesPerSample
	return d, nil
}
