package grib

import (
	"fmt"

	"github.com/pspoerri/go-tiled-eccodes/internal/bswap"
	"github.com/pspoerri/go-tiled-eccodes/section"
)

// Index walks the mmapped GRIB byte stream and returns a slice of Messages,
// one per logical data field. A single physical GRIB2 message can encode
// multiple fields by repeating sections 2..7 with shared earlier sections;
// this function fans those out so each emitted Message carries the precise
// (S0,S1,S3,S4,S5,S6,S7) tuple needed to decode it.
//
// All slices in the returned Messages alias data; nothing is copied.
func Index(data []byte) ([]*Message, error) {
	var msgs []*Message
	for off := 0; off < len(data); {
		// Find next "GRIB" magic. Most files have it at offset 0 with no
		// padding, but some sources embed leading bytes (TDF wrappers, etc.)
		// — scanning a few bytes at the start is cheap and robust.
		i := indexGRIB(data[off:])
		if i < 0 {
			break
		}
		off += i

		s0, total, err := readSection0(data[off:])
		if err != nil {
			return nil, fmt.Errorf("at offset %d: %w", off, err)
		}
		if int(total) > len(data)-off {
			return nil, fmt.Errorf("at offset %d: %w", off, ErrTruncated)
		}
		msgBytes := data[off : off+int(total)]
		batch, err := splitMessage(msgBytes, s0, off)
		if err != nil {
			return nil, fmt.Errorf("at offset %d: %w", off, err)
		}
		msgs = append(msgs, batch...)
		off += int(total)
	}
	return msgs, nil
}

func indexGRIB(b []byte) int {
	for i := 0; i+4 <= len(b); i++ {
		if b[i] == 'G' && b[i+1] == 'R' && b[i+2] == 'I' && b[i+3] == 'B' {
			return i
		}
	}
	return -1
}

func readSection0(b []byte) (section.Section0, uint64, error) {
	if len(b) < section.Section0Size {
		return section.Section0{}, 0, ErrTruncated
	}
	s0 := section.Section0{Raw: b[:section.Section0Size]}
	if s0.Magic() != "GRIB" {
		return s0, 0, ErrNotGRIB
	}
	if s0.Edition() != 2 {
		return s0, 0, ErrUnsupportedEdition
	}
	return s0, s0.TotalLength(), nil
}

// splitMessage walks sections 1..7 within a single physical GRIB message and
// emits one Message per data field (Section 7 boundary). Sections 3..6 are
// "sticky": when a later field omits a section, it inherits the previous one,
// matching the WMO repetition rules. The end marker "7777" is required.
func splitMessage(msg []byte, s0 section.Section0, fileOff int) ([]*Message, error) {
	if len(msg) < section.Section0Size+4 {
		return nil, ErrTruncated
	}
	if string(msg[len(msg)-4:]) != "7777" {
		return nil, ErrEndMarker
	}

	off := section.Section0Size
	end := len(msg) - 4

	// cur is a lock-free bag of the most recently seen sections; it never
	// carries the sync.Once state that lives on Message, so we can safely
	// snapshot it on every Section 7 without tripping go vet's copylocks.
	var (
		cur sectionBag
		out []*Message
	)
	cur.S0 = s0
	cur.fileOff = fileOff

	for off < end {
		if off+5 > end {
			return nil, ErrTruncated
		}
		ln := bswap.U32(msg, off)
		if ln < 5 || int(ln) > end-off {
			return nil, fmt.Errorf("%w: section length %d at %d", ErrBadSection, ln, off)
		}
		num := msg[off+4]
		raw := msg[off : off+int(ln)]
		switch num {
		case 1:
			cur.S1 = section.Section1{Raw: raw}
		case 2:
			cur.S2 = raw // local-use, unstructured
		case 3:
			cur.S3 = section.Section3{Raw: raw}
		case 4:
			cur.S4 = section.Section4{Raw: raw}
		case 5:
			cur.S5 = section.Section5{Raw: raw}
		case 6:
			// Indicator 254 signals "reuse the bitmap previously defined in
			// the same GRIB2 message" — the section body is empty (length 6).
			// Don't overwrite the sticky cur.S6: the decoder needs to see
			// the previously materialised bitmap so the bitmap-count maths
			// in (*Message).bitmapAndCount lines up with Section 7.
			s6 := section.Section6{Raw: raw}
			if s6.Indicator() != 254 || len(cur.S6.Raw) == 0 {
				cur.S6 = s6
			}
		case 7:
			cur.S7 = section.Section7{Raw: raw}
			// Section 7 closes one field. Materialise a Message whose lazy
			// caches start fresh.
			out = append(out, &Message{
				S0:      cur.S0,
				S1:      cur.S1,
				S2:      cur.S2,
				S3:      cur.S3,
				S4:      cur.S4,
				S5:      cur.S5,
				S6:      cur.S6,
				S7:      cur.S7,
				fileOff: cur.fileOff,
			})
		default:
			return nil, fmt.Errorf("%w: unknown section number %d at %d", ErrBadSection, num, off)
		}
		off += int(ln)
	}
	return out, nil
}

// sectionBag is the running state of splitMessage's section walk: the
// "sticky" sections inherited by each emitted Message. Mirrors the layout
// of Message but without the lazy-cache locks, so it's safe to copy.
type sectionBag struct {
	S0      section.Section0
	S1      section.Section1
	S2      []byte
	S3      section.Section3
	S4      section.Section4
	S5      section.Section5
	S6      section.Section6
	S7      section.Section7
	fileOff int
}
