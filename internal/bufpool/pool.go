// Package bufpool provides typed sync.Pool buffers keyed by size bucket.
// Used by the tile renderer to avoid per-request allocation of fx/fy and
// scratch float64/float32 slices.
package bufpool

import "sync"

type pools struct {
	f64 sync.Map // bucket -> *sync.Pool
	f32 sync.Map
}

var P pools

func bucket(n int) int {
	if n < 1 {
		return 1
	}
	b := 1
	for b < n {
		b <<= 1
	}
	return b
}

// GetF64 borrows a []float64 with len >= n. Caller must PutF64 when done.
func (p *pools) GetF64(n int) []float64 {
	bk := bucket(n)
	v, _ := p.f64.LoadOrStore(bk, &sync.Pool{New: func() any { s := make([]float64, bk); return &s }})
	pp := v.(*sync.Pool).Get().(*[]float64)
	return (*pp)[:bk]
}

func (p *pools) PutF64(s []float64) {
	bk := cap(s)
	if v, ok := p.f64.Load(bk); ok {
		s = s[:bk]
		v.(*sync.Pool).Put(&s)
	}
}

func (p *pools) GetF32(n int) []float32 {
	bk := bucket(n)
	v, _ := p.f32.LoadOrStore(bk, &sync.Pool{New: func() any { s := make([]float32, bk); return &s }})
	pp := v.(*sync.Pool).Get().(*[]float32)
	return (*pp)[:bk]
}

func (p *pools) PutF32(s []float32) {
	bk := cap(s)
	if v, ok := p.f32.Load(bk); ok {
		s = s[:bk]
		v.(*sync.Pool).Put(&s)
	}
}
