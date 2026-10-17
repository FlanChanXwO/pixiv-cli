package ugoira

import (
	"bytes"
	"testing"
)

func TestAPNGPaethRows(t *testing.T) {
	var state uint32 = 1
	next := func() byte { state = state*1664525 + 1013904223; return byte(state >> 24) }
	buffer := make([]byte, 1025)
	for n := range 2048 {
		row, previous := make([]byte, 4*(n%257)), make([]byte, 4*(n%257))
		for i := range row {
			row[i], previous[i] = next(), next()
			if n%3 == 0 {
				previous[i] = 0
			}
			if n%3 == 1 {
				previous[i] = row[i]
			}
		}
		source, prior := bytes.Clone(row), bytes.Clone(previous)
		filtered := buffer[:len(row)+1]
		for i := range filtered {
			filtered[i] = 0xff
		}
		filterAPNGRow(row, previous, filtered)
		want := make([]byte, len(row)+1)
		want[0] = 4
		for i, value := range row {
			var left, corner byte
			if i >= 4 {
				left, corner = row[i-4], previous[i-4]
			}
			want[i+1] = value - referencePNGPaeth(left, previous[i], corner)
		}
		if !bytes.Equal(filtered, want) {
			t.Fatalf("row %d: filter or buffer differs", n)
		}
		if !bytes.Equal(row, source) || !bytes.Equal(previous, prior) {
			t.Fatalf("row %d: source mutated", n)
		}
	}
}

func TestPNGPaethExhaustive(t *testing.T) {
	for left := range 256 {
		for up := range 256 {
			for corner := range 256 {
				got := pngPaeth(byte(left), byte(up), byte(corner))
				want := referencePNGPaeth(byte(left), byte(up), byte(corner))
				if got != want {
					t.Fatalf("paeth(%d,%d,%d)=%d, want %d", left, up, corner, got, want)
				}
			}
		}
	}
}

// 独立按 PNG 三绝对距离定义求解，保护阈值公式与相等时的选择顺序。
func referencePNGPaeth(left, up, corner byte) byte {
	p := int(left) + int(up) - int(corner)
	a, b, c := p-int(left), p-int(up), p-int(corner)
	if a < 0 {
		a = -a
	}
	if b < 0 {
		b = -b
	}
	if c < 0 {
		c = -c
	}
	if a <= b && a <= c {
		return left
	}
	if b <= c {
		return up
	}
	return corner
}

func BenchmarkAPNGFilter(b *testing.B) {
	for _, similar := range []bool{false, true} {
		name := "noise"
		if similar {
			name = "similar"
		}
		b.Run(name, func(b *testing.B) {
			row, previous := make([]byte, 2400), make([]byte, 2400)
			var state uint32 = 1
			for i := range row {
				state = state*1664525 + 1013904223
				row[i], previous[i] = byte(state>>24), byte(state>>16)
				if similar && i%101 != 0 {
					previous[i] = row[i]
				}
			}
			filtered := make([]byte, len(row)+1)
			for b.Loop() {
				filterAPNGRow(row, previous, filtered)
			}
		})
	}
}
