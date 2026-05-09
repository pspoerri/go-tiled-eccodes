package section

// Section6 — Bitmap.
//
//	octets 1-4 section length
//	octet  5   section number (=6)
//	octet  6   bitmap indicator (Code Table 6.0)
//	octets 7.. bitmap (1 bit per data point, MSB first)
//
// Indicator values:
//
//	0   bitmap applies and is present in this section
//	1-253 predefined bitmap (rare)
//	254   bitmap previously defined in same GRIB2 message
//	255   no bitmap applies — every value is valid
type Section6 struct {
	Raw []byte
}

func (s Section6) Indicator() uint8 { return s.Raw[5] }

// Bits returns the bitmap bytes, or nil when no bitmap is present in this
// section. For indicator 254 the caller must remember the previously seen
// bitmap.
func (s Section6) Bits() []byte {
	if len(s.Raw) <= 6 {
		return nil
	}
	return s.Raw[6:]
}
