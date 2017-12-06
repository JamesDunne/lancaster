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
	if r.Len() != 1 {
		t.Fatal("len(r) != 1")
	}
	if r.Naks()[0].start != 0 {
		t.Fatal("r[0].start != 0")
	}
	if r.Naks()[0].endEx != 10 {
		t.Fatal("r[0].endEx != 10")
	}
}

func TestNakRegions_Clear(t *testing.T) {
	r := NewNakRegions(10)
	r.Clear()
	cmp(t, r.Naks(), []Region{{start: 0, endEx: 10}})
	if r.Len() != 1 {
		t.Fatal("len(r) != 1")
	}
	if r.Naks()[0].start != 0 {
		t.Fatal("r[0].start != 0")
	}
	if r.Naks()[0].endEx != 10 {
		t.Fatal("r[0].endEx != 10")
	}
}

// [].ack(?, ?) => []
func TestNakRegions_Ack1(t *testing.T) {
	r := NewNakRegions(10)
	r.Ack(0, 10)
	cmp(t, r.Naks(), []Region{})

}

// [(0, 10)].ack(0, 10) => []
func TestNakRegions_Ack2(t *testing.T) {
	r := NewNakRegions(10)
	r.Ack(0, 10)
	cmp(t, r.Naks(), []Region{})
}

// [(0, 10)].ack(0,  5) => [(5, 10)]
func TestNakRegions_Ack3(t *testing.T) {
	r := NewNakRegions(10)
	r.Ack(0, 5)
	cmp(t, r.Naks(), []Region{{start: 5, endEx: 10}})
}

// [(0, 10)].ack(5, 10) => [(0,  5)]
func TestNakRegions_Ack4(t *testing.T) {
	r := NewNakRegions(10)
	r.Ack(5, 10)
	cmp(t, r.Naks(), []Region{{start: 0, endEx: 5}})
}

// [(0, 10)].ack(2,  5) => [(0,  2), (5, 10)]
func TestNakRegions_Ack5(t *testing.T) {
	r := NewNakRegions(10)
	r.Ack(2, 5)
	cmp(t, r.Naks(), []Region{{start: 0, endEx: 2}, {start: 5, endEx: 10}})
}

// [(0, 20)].ack(0,  5).ack(5, 10) => [(10, 20)]
func TestNakRegions_Ack6(t *testing.T) {
	r := NewNakRegions(20)
	r.Ack(0, 5)
	r.Ack(5, 10)
	cmp(t, r.Naks(), []Region{{start: 10, endEx: 20}})
}

// [(0, 20)].ack(15, 20).ack(10, 15) => [(0, 10)]
func TestNakRegions_Ack7(t *testing.T) {
	r := NewNakRegions(20)
	r.Ack(15, 20)
	r.Ack(10, 15)
	cmp(t, r.Naks(), []Region{{start: 0, endEx: 10}})
}

// [(0, 20)].ack(15, 20).ack(10, 15).ack(0, 5) => [(5, 10)]
func TestNakRegions_Ack8(t *testing.T) {
	r := NewNakRegions(20)
	r.Ack(15, 20)
	r.Ack(10, 15)
	r.Ack(0, 5)
	cmp(t, r.Naks(), []Region{{start: 5, endEx: 10}})
}

// [(0, 20)].ack(15, 19).ack(10, 15).ack(0, 5) => [(5, 10), (19, 20)]
func TestNakRegions_Ack9(t *testing.T) {
	r := NewNakRegions(20)
	r.Ack(15, 19)
	r.Ack(10, 15)
	r.Ack(0, 5)
	cmp(t, r.Naks(), []Region{{start: 5, endEx: 10}, {start: 19, endEx: 20}})
}
