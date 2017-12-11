// protocol_test.go
package main

import (
	"testing"
)

func cmp(t *testing.T, actual []Region, expected []Region) {
	if len(actual) != len(expected) {
		t.Fatalf("len(actual) != len(expected); actual = %v, expected = %v", actual, expected)
	}
	for i, a := range actual {
		e := expected[i]
		if e.start != a.start || e.endEx != a.endEx {
			t.Fatalf("actual[%d] != expected[%d]; actual = %v, expected = %v", i, i, a, e)
		}
	}
}

func TestNakRegions_Init(t *testing.T) {
	r := NewNakRegions(10)
	cmp(t, r.Naks(), []Region{{start: 0, endEx: 10}})
}

func TestNakRegions_NakAll1(t *testing.T) {
	r := NewNakRegions(10)
	r.NakAll()
	cmp(t, r.Naks(), []Region{{start: 0, endEx: 10}})
}

func TestNakRegions_NakAll2(t *testing.T) {
	r := NewNakRegions(10)
	r.NakAll()
	cmp(t, r.Acks(), []Region{})
}

// [].ack(?, ?) => []
func TestNakRegions_Ack1(t *testing.T) {
	r := NewNakRegions(10)
	r.Ack(0, 10)
	cmp(t, r.Naks(), []Region{})
	cmp(t, r.Acks(), []Region{{start: 0, endEx: 10}})
}

func TestNakRegions_Ack2(t *testing.T) {
	r := NewNakRegions(20)
	r.Ack(0, 1)
	cmp(t, r.Acks(), []Region{{start: 0, endEx: 1}})
	r.Ack(0, 2)
	cmp(t, r.Acks(), []Region{{start: 0, endEx: 2}})
}

// [(0, 10)].ack(0,  5) => [(5, 10)]
func TestNakRegions_Ack3(t *testing.T) {
	r := NewNakRegions(10)
	r.Ack(0, 5)
	cmp(t, r.Naks(), []Region{{start: 5, endEx: 10}})
	cmp(t, r.Acks(), []Region{{start: 0, endEx: 5}})
}

// [(0, 10)].ack(5, 10) => [(0,  5)]
func TestNakRegions_Ack4(t *testing.T) {
	r := NewNakRegions(10)
	r.Ack(5, 10)
	cmp(t, r.Naks(), []Region{{start: 0, endEx: 5}})
	cmp(t, r.Acks(), []Region{{start: 5, endEx: 10}})
}

// [(0, 10)].ack(2,  5) => [(0,  2), (5, 10)]
func TestNakRegions_Ack5(t *testing.T) {
	r := NewNakRegions(10)
	r.Ack(2, 5)
	cmp(t, r.Naks(), []Region{{start: 0, endEx: 2}, {start: 5, endEx: 10}})
	cmp(t, r.Acks(), []Region{{start: 2, endEx: 5}})
}

// [(0, 20)].ack(0,  5).ack(5, 10) => [(10, 20)]
func TestNakRegions_Ack6(t *testing.T) {
	r := NewNakRegions(20)
	r.Ack(0, 5)
	r.Ack(5, 10)
	cmp(t, r.Naks(), []Region{{start: 10, endEx: 20}})
	cmp(t, r.Acks(), []Region{{start: 0, endEx: 10}})
}

// [(0, 20)].ack(15, 20).ack(10, 15) => [(0, 10)]
func TestNakRegions_Ack7(t *testing.T) {
	r := NewNakRegions(20)
	r.Ack(15, 20)
	r.Ack(10, 15)
	cmp(t, r.Naks(), []Region{{start: 0, endEx: 10}})
	cmp(t, r.Acks(), []Region{{start: 10, endEx: 20}})
}

// [(0, 20)].ack(15, 20).ack(10, 15).ack(0, 5) => [(5, 10)]
func TestNakRegions_Ack8(t *testing.T) {
	r := NewNakRegions(20)
	r.Ack(15, 20)
	r.Ack(10, 15)
	r.Ack(0, 5)
	cmp(t, r.Naks(), []Region{{start: 5, endEx: 10}})
	cmp(t, r.Acks(), []Region{{start: 0, endEx: 5}, {start: 10, endEx: 20}})
}

// [(0, 20)].ack(15, 19).ack(10, 15).ack(0, 5) => [(5, 10), (19, 20)]
func TestNakRegions_Ack9(t *testing.T) {
	r := NewNakRegions(20)
	r.Ack(15, 19)
	r.Ack(10, 15)
	r.Ack(0, 5)
	cmp(t, r.Naks(), []Region{{start: 5, endEx: 10}, {start: 19, endEx: 20}})
	cmp(t, r.Acks(), []Region{{start: 0, endEx: 5}, {start: 10, endEx: 19}})
}

// [(5, 10)].nak(0,  10) => [(0, 10)]
func TestNakRegions_Nak1(t *testing.T) {
	r := NewNakRegions(10)
	r.Ack(0, 5)
	r.Nak(0, 10)
	cmp(t, r.Naks(), []Region{{start: 0, endEx: 10}})
	cmp(t, r.Acks(), []Region{})
}

// [(5, 20)].nak(0,  15) => [(0, 15)]
func TestNakRegions_Nak2(t *testing.T) {
	r := NewNakRegions(20)
	r.Ack(0, 5)
	r.Nak(0, 15)
	cmp(t, r.Naks(), []Region{{start: 0, endEx: 20}})
	cmp(t, r.Acks(), []Region{})
}

// [(5, 10), (15, 20)].nak(2,  12) => [(2, 12), (15, 20)]
func TestNakRegions_Nak3(t *testing.T) {
	r := NewNakRegions(20)
	r.Ack(0, 5)
	r.Ack(10, 15)
	r.Nak(2, 12)
	cmp(t, r.Naks(), []Region{{start: 2, endEx: 12}, {start: 15, endEx: 20}})
	cmp(t, r.Acks(), []Region{{start: 0, endEx: 2}, {start: 12, endEx: 15}})
}

// [(5, 10), (15, 20)].nak(0,  15) => [(0, 20)]
func TestNakRegions_Nak4(t *testing.T) {
	r := NewNakRegions(20)
	r.Ack(0, 5)
	r.Ack(10, 15)
	r.Nak(0, 15)
	cmp(t, r.Naks(), []Region{{start: 0, endEx: 20}})
	cmp(t, r.Acks(), []Region{})
}

// [(5, 10), (15, 20)].nak(0,  20) => [(0, 20)]
func TestNakRegions_Nak5(t *testing.T) {
	r := NewNakRegions(20)
	r.Ack(0, 5)
	r.Ack(10, 15)
	r.Nak(0, 20)
	cmp(t, r.Naks(), []Region{{start: 0, endEx: 20}})
	cmp(t, r.Acks(), []Region{})
}
