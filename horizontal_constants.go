package grib

import (
	"errors"
	"fmt"
	"math"
	"sync"

	"github.com/pspoerri/go-tiled-eccodes/grid"
)

// HorizontalConstants is a UUID-keyed catalogue of icosahedral cell
// coordinates loaded from a companion GRIB2 file. Pass to File.AttachCoordinates
// (or call CoordinatesFor + SetGridCoordinates yourself) to wire each
// unstructured grid in a forecast file to its mesh.
//
// A typical workflow:
//
//	hc, err := grib.LoadHorizontalConstants("horizontal_constants_icon-d2.grib2")
//	f, err := grib.Open("icon-d2_t_2m.grib2")
//	if _, err := f.AttachCoordinates(hc); err != nil { ... }
//	// Now all messages on the matching icosahedral grid render correctly.
//
// Detection: this loader pairs unstructured-grid messages by their UUID. For
// each pair on the same UUID, the message whose value range fits a latitude
// (max |v| ≤ ~π/2 in radians or ≤ 90 in degrees) becomes the lat array; the
// other becomes the lon array. The unit is auto-detected — DWD ICON ships
// radians, MeteoSwiss ICON-CH ships degrees — and the output is always in
// degrees.
type HorizontalConstants struct {
	byUUID map[[16]byte]*coordEntry
}

type coordEntry struct {
	lats []float64
	lons []float64
	// First message we hand out via SetGridCoordinates "owns" the KD-tree;
	// subsequent messages share it via SetCoordinatesFrom. nil until first
	// AttachCoordinates on a matching unstructured grid.
	//
	// mu serialises the "is primary set yet?" check across concurrent
	// AttachCoordinates calls (different *grib.File instances sharing
	// one HorizontalConstants — the gribstore.Reader pattern).
	// Without it, two parallel openFile calls could both see primary=nil
	// and each end up "primary" with their own KD-tree, defeating the
	// per-mesh sharing and doubling memory.
	mu      sync.Mutex
	primary *grid.Unstructured
}

// CoordinatesFor returns the (lats, lons) arrays for the given UUID, or
// nil if no matching mesh was loaded.
func (h *HorizontalConstants) CoordinatesFor(uuid [16]byte) (lats, lons []float64) {
	if h == nil {
		return nil, nil
	}
	e, ok := h.byUUID[uuid]
	if !ok {
		return nil, nil
	}
	return e.lats, e.lons
}

// UUIDs returns the set of grid UUIDs catalogued in this HorizontalConstants.
// Order is unspecified.
func (h *HorizontalConstants) UUIDs() [][16]byte {
	if h == nil {
		return nil
	}
	out := make([][16]byte, 0, len(h.byUUID))
	for k := range h.byUUID {
		out = append(out, k)
	}
	return out
}

// LoadHorizontalConstants opens path, decodes every GRIB2 message in it on
// an unstructured grid (template 3.101), and pairs each per-UUID into a
// (lat, lon) catalogue.
//
// Returns an error if the file does not parse, contains no unstructured
// messages, or any UUID has fewer than two messages whose value ranges fit
// the lat/lon shape.
func LoadHorizontalConstants(path string) (*HorizontalConstants, error) {
	f, err := Open(path)
	if err != nil {
		return nil, fmt.Errorf("LoadHorizontalConstants %s: %w", path, err)
	}
	defer f.Close()
	return loadHorizontalConstantsFromFile(f)
}

// LoadHorizontalConstantsFromBytes is the in-memory variant of
// LoadHorizontalConstants. data must be a valid GRIB2 byte slice (one or
// more messages on template 3.101). The slice is not retained beyond the
// call — every coordinate array returned is freshly allocated.
//
// Useful when the caller is building a run from streamed HTTP downloads
// and never wants to land the GRIB on disk.
func LoadHorizontalConstantsFromBytes(data []byte) (*HorizontalConstants, error) {
	f, err := FromBytes(data)
	if err != nil {
		return nil, fmt.Errorf("LoadHorizontalConstantsFromBytes: %w", err)
	}
	defer f.Close()
	return loadHorizontalConstantsFromFile(f)
}

func loadHorizontalConstantsFromFile(f *File) (*HorizontalConstants, error) {
	type bucket struct {
		messages []*Message
		decoded  [][]float64
	}
	byUUID := map[[16]byte]*bucket{}

	for _, m := range f.Messages() {
		if m.S3.TemplateNumber() != 101 {
			continue
		}
		g, err := m.Grid()
		if err != nil {
			continue
		}
		u, ok := g.(*grid.Unstructured)
		if !ok {
			continue
		}
		vals, err := m.DecodeFloat64(nil)
		if err != nil {
			return nil, fmt.Errorf("decode message at offset %d: %w", m.FileOffset(), err)
		}
		b := byUUID[u.UUID]
		if b == nil {
			b = &bucket{}
			byUUID[u.UUID] = b
		}
		b.messages = append(b.messages, m)
		b.decoded = append(b.decoded, vals)
	}
	if len(byUUID) == 0 {
		return nil, errors.New("LoadHorizontalConstants: no unstructured-grid messages in file")
	}

	out := &HorizontalConstants{byUUID: map[[16]byte]*coordEntry{}}
	for uuid, b := range byUUID {
		lats, lons, err := pairLatLonWithMessages(b.messages, b.decoded)
		if err != nil {
			return nil, fmt.Errorf("UUID %x: %w", uuid, err)
		}
		out.byUUID[uuid] = &coordEntry{lats: lats, lons: lons}
	}
	return out, nil
}

// GRIB2 parameter identifiers for cell-center latitude / longitude as
// they ship in ICON horizontal_constants files (DWD: clat/clon,
// MeteoSwiss: tlat/tlon). Both centres encode the same WMO triple
// (discipline 0 / category 191 / numbers 1, 2) — discipline 0 is
// "meteorological products", category 191 is "miscellaneous", and
// numbers 1 and 2 are the WMO-blessed slots for "geographical
// latitude / longitude". pairLatLonWithMessages prefers these codes
// when present and only falls back to the value-range heuristic when
// they are not.
const (
	clatDiscipline = 0
	clatCategory   = 191
	clatNumber     = 1
	clonNumber     = 2
)

// pairLatLonWithMessages picks the lat/lon arrays out of a bucket of
// (message, decoded values) pairs for one UUID. Identification order:
//
//  1. WMO parameter codes — message at (disc=0, cat=191, num=1) is
//     latitude, num=2 is longitude. Reliable for every ICON ensemble
//     we ship today (DWD ICON-D2/Global, MeteoSwiss ICON-CH).
//  2. Value-range heuristic — for non-ICON unstructured grids that
//     might bundle clat/clon under different parameter codes.
//
// Either path produces output in degrees; values stored as radians are
// converted on the way out.
func pairLatLonWithMessages(messages []*Message, decoded [][]float64) ([]float64, []float64, error) {
	if len(messages) != len(decoded) {
		return nil, nil, errors.New("messages/decoded length mismatch")
	}
	if len(decoded) < 2 {
		return nil, nil, errors.New("need at least two messages per UUID (clat + clon)")
	}

	// Step 1: look up by WMO parameter code.
	latIdx, lonIdx := -1, -1
	for i, m := range messages {
		if m.S0.Discipline() != clatDiscipline {
			continue
		}
		if m.S4.ParameterCategory() != clatCategory {
			continue
		}
		switch m.S4.ParameterNumber() {
		case clatNumber:
			if latIdx < 0 {
				latIdx = i
			}
		case clonNumber:
			if lonIdx < 0 {
				lonIdx = i
			}
		}
	}
	if latIdx >= 0 && lonIdx >= 0 {
		lats := maybeRadiansToDegrees(decoded[latIdx])
		lons := maybeRadiansToDegrees(decoded[lonIdx])
		return lats, lons, nil
	}

	// Step 2: fall back to the legacy value-range heuristic.
	return pairLatLon(decoded)
}

// maybeRadiansToDegrees auto-detects whether v is stored in radians
// (max-abs ≤ π+ε) and converts it to degrees if so. Used after the
// parameter-code lookup, where we know the values *are* lat or lon
// but not which unit. ICON files come in both flavours: DWD ships
// radians, MeteoSwiss ships degrees.
func maybeRadiansToDegrees(v []float64) []float64 {
	maxAbs := 0.0
	for _, x := range v {
		if math.IsNaN(x) {
			continue
		}
		a := x
		if a < 0 {
			a = -a
		}
		if a > maxAbs {
			maxAbs = a
		}
	}
	if maxAbs <= math.Pi+0.01 {
		return radiansToDegrees(v)
	}
	return v
}

// pairLatLon picks the lat and lon arrays out of a bucket of decoded value
// arrays for the same UUID. Heuristic: classify each array by the absolute
// maximum of its non-NaN values:
//
//	max ≤ π/2+ε   →  latitude in radians
//	π/2+ε < max ≤ π+ε →  longitude in radians
//	max ≤ 90+ε   →  latitude in degrees
//	max ≤ 360+ε  →  longitude in degrees
//
// Output is always degrees (radians are converted in place).
func pairLatLon(decoded [][]float64) ([]float64, []float64, error) {
	if len(decoded) < 2 {
		return nil, nil, errors.New("need at least two messages per UUID (clat + clon)")
	}
	type classified struct {
		idx   int
		isLat bool
		isLon bool
		isRad bool
	}
	var entries []classified
	for i, v := range decoded {
		maxAbs := 0.0
		for _, x := range v {
			if math.IsNaN(x) {
				continue
			}
			a := x
			if a < 0 {
				a = -a
			}
			if a > maxAbs {
				maxAbs = a
			}
		}
		c := classified{idx: i}
		switch {
		case maxAbs <= math.Pi/2+0.01:
			c.isLat = true
			c.isRad = true
		case maxAbs <= math.Pi+0.01:
			c.isLon = true
			c.isRad = true
		case maxAbs <= 90+0.5:
			c.isLat = true
		case maxAbs <= 360+0.5:
			c.isLon = true
		default:
			// Doesn't fit either; skip.
			continue
		}
		entries = append(entries, c)
	}
	var latIdx, lonIdx = -1, -1
	var latRad, lonRad bool
	for _, e := range entries {
		if e.isLat && latIdx < 0 {
			latIdx = e.idx
			latRad = e.isRad
		} else if e.isLon && lonIdx < 0 {
			lonIdx = e.idx
			lonRad = e.isRad
		}
	}
	if latIdx < 0 || lonIdx < 0 {
		return nil, nil, errors.New("could not identify lat/lon messages")
	}
	lats := decoded[latIdx]
	lons := decoded[lonIdx]
	if latRad {
		lats = radiansToDegrees(lats)
	}
	if lonRad {
		lons = radiansToDegrees(lons)
	}
	return lats, lons, nil
}

// radiansToDegrees returns a freshly allocated copy of v with each non-NaN
// entry scaled by 180/π. Allocates rather than mutating in place so that
// the caller's input slice (typically returned from a Message decode cache)
// is not silently rewritten.
func radiansToDegrees(v []float64) []float64 {
	const r2d = 180.0 / math.Pi
	out := make([]float64, len(v))
	for i, x := range v {
		if math.IsNaN(x) {
			out[i] = x
		} else {
			out[i] = x * r2d
		}
	}
	return out
}

// AttachCoordinates walks every message in this file. For each message on
// an unstructured grid (template 3.101) whose UUID is present in hc, it
// attaches the matching (lat, lon) arrays. Returns the number of messages
// successfully wired.
//
// The KD-tree is built once per UUID (on the first matching message) and
// shared across all subsequent messages on the same mesh — saving ~32 N
// bytes plus an O(N log N) build per redundant tree.
func (f *File) AttachCoordinates(hc *HorizontalConstants) (int, error) {
	if hc == nil {
		return 0, errors.New("nil HorizontalConstants")
	}
	count := 0
	for _, m := range f.Messages() {
		if m.S3.TemplateNumber() != 101 {
			continue
		}
		g, err := m.Grid()
		if err != nil {
			continue
		}
		u, ok := g.(*grid.Unstructured)
		if !ok {
			continue
		}
		entry, ok := hc.byUUID[u.UUID]
		if !ok {
			continue
		}
		if u.HasCoordinates() {
			continue
		}
		entry.mu.Lock()
		if entry.primary == nil {
			if err := u.SetCoordinates(entry.lats, entry.lons); err != nil {
				entry.mu.Unlock()
				return count, fmt.Errorf("attach UUID %x: %w", u.UUID, err)
			}
			entry.primary = u
			entry.mu.Unlock()
		} else {
			primary := entry.primary
			entry.mu.Unlock()
			if err := u.SetCoordinatesFrom(primary); err != nil {
				return count, fmt.Errorf("share UUID %x: %w", u.UUID, err)
			}
		}
		count++
	}
	return count, nil
}
