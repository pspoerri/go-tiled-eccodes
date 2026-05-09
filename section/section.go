package section

import "github.com/pspoerri/go-tiled-eccodes/internal/bswap"

// Length reads a section's 4-byte length prefix. Returns 0 if the slice is
// too short.
func Length(b []byte) uint32 {
	if len(b) < 4 {
		return 0
	}
	return bswap.U32(b, 0)
}

// Number reads a section's section-number byte (octet 5 = byte 4). Sections
// 0 and 8 do not follow this layout.
func Number(b []byte) uint8 {
	if len(b) < 5 {
		return 0
	}
	return b[4]
}
