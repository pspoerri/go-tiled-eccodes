package aec

import "testing"

func TestGetBitsMSB(t *testing.T) {
	// 0xB4 = 1011_0100, 0x2D = 0010_1101.
	b := bitReader{src: []byte{0xB4, 0x2D}}
	// Read 3 bits: 101 = 5.
	if v, ok := b.getBits(3); !ok || v != 0b101 {
		t.Fatalf("getBits(3) = %d,%v want 5,true", v, ok)
	}
	// Next 5 bits: 10100 = 20.
	if v, ok := b.getBits(5); !ok || v != 0b10100 {
		t.Fatalf("getBits(5) = %d,%v want 20,true", v, ok)
	}
	// Next 8 bits span into byte 2: 00101101 = 0x2D.
	if v, ok := b.getBits(8); !ok || v != 0x2D {
		t.Fatalf("getBits(8) = %d,%v want 45,true", v, ok)
	}
	if _, ok := b.getBits(1); ok {
		t.Fatalf("getBits past end should fail")
	}
}

func TestGetBitsZero(t *testing.T) {
	b := bitReader{src: []byte{0xFF}}
	if v, ok := b.getBits(0); !ok || v != 0 {
		t.Fatalf("getBits(0) = %d,%v want 0,true", v, ok)
	}
}

func TestGetFS(t *testing.T) {
	// 0001_1000: FS values: 3 (000 then 1), then 0 (1), then bits 000 left.
	b := bitReader{src: []byte{0b0001_1000}}
	if v, ok := b.getFS(); !ok || v != 3 {
		t.Fatalf("getFS#1 = %d,%v want 3,true", v, ok)
	}
	if v, ok := b.getFS(); !ok || v != 0 {
		t.Fatalf("getFS#2 = %d,%v want 0,true", v, ok)
	}
	// Remaining 000 with no terminating 1 -> exhausted.
	if _, ok := b.getFS(); ok {
		t.Fatalf("getFS#3 should fail (no terminating 1)")
	}
}

func TestGetFSAcrossBytes(t *testing.T) {
	// 12 leading zeros then 1: 0x00, 0x08 = 0000_0000 0000_1000.
	b := bitReader{src: []byte{0x00, 0x08}}
	if v, ok := b.getFS(); !ok || v != 12 {
		t.Fatalf("getFS = %d,%v want 12,true", v, ok)
	}
}
