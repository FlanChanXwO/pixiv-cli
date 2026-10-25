package ugoira

import (
	"math"
	"testing"
)

func TestLinearToSRGB(t *testing.T) {
	for i, x := range srgbToLinear {
		if got := linearToSRGB(x); int(got) != i {
			t.Errorf("linear round trip %d: %d", i, got)
		}
	}
	for _, tc := range []struct {
		x    float32
		want byte
	}{{-1, 0}, {0, 0}, {1, 255}, {2, 255}, {float32(math.Inf(1)), 255}, {float32(math.Inf(-1)), 0}, {float32(math.NaN()), 0}} {
		if got := linearToSRGB(tc.x); got != tc.want {
			t.Errorf("linear %v: %d, want %d", tc.x, got, tc.want)
		}
	}
}
