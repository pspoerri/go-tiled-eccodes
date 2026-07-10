package grib

// SetPredefinedBitmap attaches the centre-defined bitmap referenced by this
// message's Section 6 indicator (1..253). GRIB2 does not define the contents
// of these bitmaps globally; callers must obtain them from the originating
// centre. Bits are ordered most-significant first, just like an inline bitmap.
//
// Registration must happen before the first decode or render. The bitmap is
// copied, so the caller may reuse its buffer after this method returns.
func (m *Message) SetPredefinedBitmap(bitmap []byte) error {
	indicator := m.S6.Indicator()
	if indicator < 1 || indicator > 253 {
		return ErrUnsupportedTemplate
	}
	g, err := m.Grid()
	if err != nil {
		return err
	}
	nbytes := (g.NumPoints() + 7) >> 3
	if len(bitmap) < nbytes {
		return ErrTruncated
	}
	registered := append([]byte(nil), bitmap[:nbytes]...)

	m.bitmapMu.Lock()
	defer m.bitmapMu.Unlock()
	if m.decodeStarted {
		return ErrDecodeStarted
	}
	m.predefinedBitmap = registered
	return nil
}

// SetPredefinedBitmap registers a centre-defined bitmap on every message in
// the file that references indicator. It returns the number of matching
// messages. Registration should be completed before any matching message is
// decoded; if a matching decode has started, the method returns
// ErrDecodeStarted after registering any earlier matches.
func (f *File) SetPredefinedBitmap(indicator uint8, bitmap []byte) (int, error) {
	if indicator < 1 || indicator > 253 {
		return 0, ErrUnsupportedTemplate
	}
	matched := 0
	for _, m := range f.messages {
		if m.S6.Indicator() != indicator {
			continue
		}
		if err := m.SetPredefinedBitmap(bitmap); err != nil {
			return matched, err
		}
		matched++
	}
	return matched, nil
}
