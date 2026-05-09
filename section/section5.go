package section

import "github.com/pspoerri/go-tiled-eccodes/internal/bswap"

// Section5 — Data Representation.
//
//	octets 1-4   section length
//	octet  5     section number (=5)
//	octets 6-9   number of data points encoded
//	octets 10-11 data representation template number
//	octets 12..  template
type Section5 struct {
	Raw []byte
}

func (s Section5) NumDataPoints() uint32 { return bswap.U32(s.Raw, 5) }
func (s Section5) TemplateNumber() uint16 {
	return bswap.U16(s.Raw, 9)
}
func (s Section5) Template() []byte { return s.Raw[11:] }
