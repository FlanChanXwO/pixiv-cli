package ugoira

import (
	"bytes"
	"testing"
)

func TestAPNGSubRows(t *testing.T) {
	var state uint32 = 1
	next := func() byte { state = state*1664525 + 1013904223; return byte(state >> 24) }
	buffer := make([]byte, 1025)
	for n := range 2048 {
		row := make([]byte, 4*(n%257))
		for i := range row {
			row[i] = next()
			if n%3 == 0 {
				row[i] = 0
			}
			if n%3 == 1 {
				row[i] = byte(i % 4)
			}
		}
		source := bytes.Clone(row)
		filtered := buffer[:len(row)+1]
		for i := range filtered {
			filtered[i] = 0xff
		}
		filterAPNGRow(row, filtered)
		want := make([]byte, len(row)+1)
		want[0] = 1
		for i, value := range row {
			var left byte
			if i >= 4 {
				left = row[i-4]
			}
			want[i+1] = value - left
		}
		if !bytes.Equal(filtered, want) {
			t.Fatalf("row %d: filter or buffer differs", n)
		}
		if !bytes.Equal(row, source) {
			t.Fatalf("row %d: source mutated", n)
		}
	}
}

func BenchmarkAPNGFilter(b *testing.B) {
	row := make([]byte, 2400)
	var state uint32 = 1
	for i := range row {
		state = state*1664525 + 1013904223
		row[i] = byte(state >> 24)
	}
	filtered := make([]byte, len(row)+1)
	for b.Loop() {
		filterAPNGRow(row, filtered)
	}
}
