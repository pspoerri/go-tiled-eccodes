package decode

import (
	"fmt"

	"github.com/pspoerri/go-tiled-eccodes/aec"
)

// ccsdsDecode decodes the CCSDS/AEC Section-7 bitstream into output using the
// pure-Go aec package. output must be sized numPoints*bytesPerSample; the GRIB
// flag byte maps directly onto aec.Flags. Mirrors the old libaec call site.
func ccsdsDecode(input, output []byte, bitsPerSample int, blockSize, rsi, flags uint) error {
	if len(input) == 0 {
		return fmt.Errorf("decode: empty CCSDS input")
	}
	n, err := aec.Decode(output, input, aec.Config{
		BitsPerSample: bitsPerSample,
		BlockSize:     int(blockSize),
		RSI:           int(rsi),
		Flags:         aec.Flags(flags),
	})
	if err != nil {
		return fmt.Errorf("decode: CCSDS: %w", err)
	}
	if n != len(output) {
		return fmt.Errorf("decode: CCSDS produced %d bytes, expected %d", n, len(output))
	}
	return nil
}
