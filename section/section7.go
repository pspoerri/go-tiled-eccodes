package section

// Section7 — Data.
//
//	octets 1-4 section length
//	octet  5   section number (=7)
//	octets 6.. encoded data values (interpretation depends on Section 5
//	           template)
type Section7 struct {
	Raw []byte
}

// Payload returns the encoded data bytes (excluding the 5-byte header).
func (s Section7) Payload() []byte { return s.Raw[5:] }
