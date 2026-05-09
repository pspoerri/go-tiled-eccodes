// Package section contains zero-copy parsers for GRIB2 sections. Each section
// type wraps a []byte view into the mmapped file; parsed fields are computed
// on access via small methods so we never copy the source bytes.
package section

import (
	"github.com/pspoerri/go-tiled-eccodes/internal/bswap"
)

// Section0 — Indicator. Fixed 16 bytes.
//
//	octets 1-4   "GRIB"
//	octet  5     reserved
//	octet  6     reserved
//	octet  7     discipline (Code Table 0.0)
//	octet  8     edition number
//	octets 9-16  total length of GRIB message in octets (uint64 BE)
type Section0 struct {
	Raw []byte
}

func (s Section0) Magic() string     { return string(s.Raw[0:4]) }
func (s Section0) Discipline() uint8 { return s.Raw[6] }
func (s Section0) Edition() uint8    { return s.Raw[7] }
func (s Section0) TotalLength() uint64 {
	return bswap.U64(s.Raw, 8)
}

const Section0Size = 16
