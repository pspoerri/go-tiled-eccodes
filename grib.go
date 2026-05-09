// Package grib provides a pure-Go GRIB2 decoder optimized for serving WGS84
// tiles in a weather API. It assumes the source file is mmapped and exposes
// zero-copy section parsers, typed buffer renderers (float32/float64 and
// signed/unsigned 8/16/32/64-bit ints), and three sampling modes (nearest,
// bicubic, mode).
//
// Scope: GRIB2 only. Decode only. See plan.md for the full design.
package grib

import (
	"sync"

	"github.com/pspoerri/go-tiled-eccodes/grid"
	imap "github.com/pspoerri/go-tiled-eccodes/internal/mmap"
	"github.com/pspoerri/go-tiled-eccodes/section"
)

// File is a parsed view of a mmapped GRIB2 file. The underlying mapping is
// kept alive for the lifetime of the File and released by Close.
type File struct {
	data     []byte
	owns     bool       // true if we own the mmap and must munmap on Close
	messages []*Message // indexed at Open time
}

// Open mmaps path read-only and indexes its GRIB2 messages.
func Open(path string) (*File, error) {
	data, err := imap.Open(path)
	if err != nil {
		return nil, err
	}
	msgs, err := Index(data)
	if err != nil {
		_ = imap.Close(data)
		return nil, err
	}
	imap.AdviseRandom(data)
	return &File{data: data, owns: true, messages: msgs}, nil
}

// FromBytes wraps an existing byte slice (e.g. caller-managed mmap or in-memory
// buffer). The caller retains ownership of the slice; Close will not unmap it.
func FromBytes(data []byte) (*File, error) {
	msgs, err := Index(data)
	if err != nil {
		return nil, err
	}
	return &File{data: data, owns: false, messages: msgs}, nil
}

// Close unmaps the file (if owned) and releases cached state.
func (f *File) Close() error {
	for _, m := range f.messages {
		m.releaseCache()
	}
	if f.owns {
		err := imap.Close(f.data)
		f.data = nil
		return err
	}
	return nil
}

// Messages returns the indexed messages. The slice and underlying entries are
// owned by the File; do not retain across Close.
func (f *File) Messages() []*Message { return f.messages }

// Message represents one logical GRIB2 data field — the (S0,S1,S3,S4,S5,S6,S7)
// tuple needed to interpret a single decoded grid. Multiple Messages may share
// underlying section bytes (when one physical GRIB2 message packs multiple
// fields with the same grid).
type Message struct {
	S0 section.Section0
	S1 section.Section1
	S2 []byte // optional local-use section (unstructured)
	S3 section.Section3
	S4 section.Section4
	S5 section.Section5
	S6 section.Section6
	S7 section.Section7

	fileOff int

	// Cache for repeated ValueAt / Render calls on the same message: a single
	// decoded []float64 buffer the size of the grid. Not allocated until first
	// use; protected by once so concurrent renderers race to the same result.
	once   sync.Once
	cached []float64
	decErr error

	// Grid is parsed and cached once so callers can attach per-grid state
	// (e.g. SetGridCoordinates on an unstructured grid) and have it persist
	// across subsequent renders.
	gridOnce sync.Once
	grid     grid.Grid
	gridErr  error
}

// FileOffset is the byte offset of this message's GRIB indicator within the
// source file (or input slice). Useful for diagnostics and for storing
// per-message offsets in an external index.
func (m *Message) FileOffset() int { return m.fileOff }

func (m *Message) releaseCache() {
	m.cached = nil
}
