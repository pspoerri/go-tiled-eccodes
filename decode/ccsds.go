package decode

import (
	"encoding/binary"
	"fmt"
	"math"

	"github.com/pspoerri/go-tiled-eccodes/internal/bswap"
)

// CCSDS decodes Data Representation Template 5.42 (CCSDS 121.0-B-3
// adaptive entropy coding, the same algorithm libaec implements).
//
// Template 5.42 layout (template bytes — i.e. starting at section 5 byte 11):
//
//	bytes 0-3   reference value R (IEEE-754 float32 BE)
//	bytes 4-5   binary scale factor E (sign-magnitude int16)
//	bytes 6-7   decimal scale factor D (sign-magnitude int16)
//	byte  8     number of bits per packed value
//	byte  9     type of original field values (Code Table 5.1)
//	byte  10    CCSDS compression options mask (passes through to aec.Flags)
//	byte  11    CCSDS block size
//	bytes 12-13 CCSDS reference sample interval (uint16 BE)
//
// Decoded value: Y = (R + X * 2^E) / 10^D
//
// The Section 7 payload is the raw CCSDS/AEC bitstream; this function decodes
// it with the pure-Go aec package (a port of libaec) — no CGo required.
func CCSDS(template, data []byte, numPoints int, dst []float64) ([]float64, error) {
	if len(template) < 14 {
		return nil, fmt.Errorf("decode: template 5.42 too short (%d bytes)", len(template))
	}
	r := bswap.F32(template, 0)
	e := bswap.I16SM(template, 4)
	d := bswap.I16SM(template, 6)
	nbits := int(template[8])
	flags := uint(template[10])
	blockSize := uint(template[11])
	rsi := uint(binary.BigEndian.Uint16(template[12:14]))

	if cap(dst) < numPoints {
		dst = make([]float64, numPoints)
	} else {
		dst = dst[:numPoints]
	}

	scaleBin := math.Ldexp(1, int(e))
	scaleDec := math.Pow10(-int(d))
	bias := float64(r) * scaleDec

	if nbits == 0 {
		// Constant field: every value equals R/10^D, no payload to decode.
		v := bias
		for i := range dst {
			dst[i] = v
		}
		return dst, nil
	}

	const (
		aecDataSigned = 0x01
		aecData3Byte  = 0x02
		aecDataMSB    = 0x04
	)
	signed := flags&aecDataSigned != 0
	msbFirst := flags&aecDataMSB != 0
	bytesPerSample := bytesPerSampleFromBits(nbits, flags)
	rawOut := make([]byte, numPoints*bytesPerSample)
	if err := ccsdsDecode(data, rawOut, nbits, blockSize, rsi, flags); err != nil {
		return nil, err
	}

	// Convert raw integer samples → physical Y. libaec's byte order is
	// controlled by the AEC_DATA_MSB flag (big-endian when set, little-
	// endian otherwise) — *not* host endianness, despite what the field
	// width suggests. The 3-byte (24-bit) variant is always big-endian
	// per the CCSDS standard.
	mul := scaleBin * scaleDec
	switch bytesPerSample {
	case 1:
		for i, b := range rawOut {
			x := float64(b)
			if signed {
				x = float64(int8(b))
			}
			dst[i] = bias + x*mul
		}
	case 2:
		for i := 0; i < numPoints; i++ {
			off := i * 2
			var u uint16
			if msbFirst {
				u = uint16(rawOut[off])<<8 | uint16(rawOut[off+1])
			} else {
				u = uint16(rawOut[off]) | uint16(rawOut[off+1])<<8
			}
			x := float64(u)
			if signed {
				x = float64(int16(u))
			}
			dst[i] = bias + x*mul
		}
	case 3:
		// 24-bit samples (AEC_DATA_3BYTE). Always MSB-first per CCSDS.
		for i := 0; i < numPoints; i++ {
			off := i * 3
			u := uint32(rawOut[off])<<16 | uint32(rawOut[off+1])<<8 | uint32(rawOut[off+2])
			x := float64(u)
			if signed && u&0x800000 != 0 {
				x = float64(int32(u | 0xff000000))
			}
			dst[i] = bias + x*mul
		}
	case 4:
		for i := 0; i < numPoints; i++ {
			off := i * 4
			var u uint32
			if msbFirst {
				u = uint32(rawOut[off])<<24 | uint32(rawOut[off+1])<<16 |
					uint32(rawOut[off+2])<<8 | uint32(rawOut[off+3])
			} else {
				u = uint32(rawOut[off]) | uint32(rawOut[off+1])<<8 |
					uint32(rawOut[off+2])<<16 | uint32(rawOut[off+3])<<24
			}
			x := float64(u)
			if signed {
				x = float64(int32(u))
			}
			dst[i] = bias + x*mul
		}
	default:
		return nil, fmt.Errorf("decode: unexpected sample width %d", bytesPerSample)
	}
	return dst, nil
}

// bytesPerSampleFromBits mirrors libaec's storage-width selection: 1, 2,
// or 4 bytes by default, with the optional AEC_DATA_3BYTE flag forcing 3
// bytes for 17–24 bit-per-sample data.
func bytesPerSampleFromBits(nbits int, flags uint) int {
	const aecData3Byte = 2
	switch {
	case nbits <= 8:
		return 1
	case nbits <= 16:
		return 2
	case flags&aecData3Byte != 0 && nbits <= 24:
		return 3
	default:
		return 4
	}
}
