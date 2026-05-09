package decode

import "sync"

// Reusable []uint32 working buffers, bucketed by next power of two. Callers
// borrow with get(n) and return with put. The pool grows transparently — a
// renderer that hits ~1M points repeatedly amortizes allocs to ~zero.
type u32Pool struct{ pools sync.Map } // size bucket -> *sync.Pool

var unpackPool u32Pool
var i64WorkPool i64Pool

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

func (p *u32Pool) get(n int) []uint32 {
	bk := bucket(n)
	v, _ := p.pools.LoadOrStore(bk, &sync.Pool{New: func() any { s := make([]uint32, bk); return &s }})
	pp := v.(*sync.Pool).Get().(*[]uint32)
	return (*pp)[:bk]
}

func (p *u32Pool) put(s []uint32) {
	bk := cap(s)
	if v, ok := p.pools.Load(bk); ok {
		s = s[:bk]
		v.(*sync.Pool).Put(&s)
	}
}

// i64Pool mirrors u32Pool for the int64 working buffer that holds the raw
// (X1[g] + X2[k]) integers between decodeComplexInto and finalizeValues.
type i64Pool struct{ pools sync.Map }

func (p *i64Pool) get(n int) []int64 {
	bk := bucket(n)
	v, _ := p.pools.LoadOrStore(bk, &sync.Pool{New: func() any { s := make([]int64, bk); return &s }})
	pp := v.(*sync.Pool).Get().(*[]int64)
	return (*pp)[:bk]
}

func (p *i64Pool) put(s []int64) {
	bk := cap(s)
	if v, ok := p.pools.Load(bk); ok {
		s = s[:bk]
		v.(*sync.Pool).Put(&s)
	}
}
