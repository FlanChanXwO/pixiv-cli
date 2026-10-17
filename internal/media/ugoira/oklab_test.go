package ugoira

import (
	"encoding/json"
	"math"
	"os"
	"testing"
)

type quantizationReference struct {
	Conversion struct {
		Input  []rgb
		Bits   [][3]uint32
		Output []rgb
	}
	Clamp struct {
		Bits   [][3]uint32
		Output []rgb
	}
	Reservoir struct {
		Pixels, Length  int
		OpaquePixels    uint64 `json:"opaque_pixels"`
		HasTransparency bool   `json:"has_transparency"`
		State           uint64
		SHA256          string
	}
	Cases []struct {
		Name            string
		Input, Colors   []rgb
		Bits            [][3]uint32
		Counts, Indices []uint32
		Palettes        []struct {
			Size int
			Bits [][3]uint32
			RGB  []rgb
		}
	}
}

func readQuantizationReference(t *testing.T) quantizationReference {
	t.Helper()
	data, err := os.ReadFile("testdata/quantization.json")
	if err != nil {
		t.Fatal(err)
	}
	var ref quantizationReference
	if err := json.Unmarshal(data, &ref); err != nil {
		t.Fatal(err)
	}
	if len(ref.Conversion.Input) != 2048 || len(ref.Cases) != 5 {
		t.Fatal("incomplete old encoder reference")
	}
	return ref
}

func oklabBits(c oklab) [3]uint32 {
	return [3]uint32{math.Float32bits(c[0]), math.Float32bits(c[1]), math.Float32bits(c[2])}
}

func TestOklabReference(t *testing.T) {
	ref := readQuantizationReference(t)
	if len(ref.Conversion.Input) != len(ref.Conversion.Bits) || len(ref.Conversion.Input) != len(ref.Conversion.Output) {
		t.Fatal("conversion reference length mismatch")
	}
	for i, c := range ref.Conversion.Input {
		got := srgbToOklab(c)
		if bits := oklabBits(got); bits != ref.Conversion.Bits[i] {
			t.Fatalf("color %d %v: bits %x, old %x", i, c, bits, ref.Conversion.Bits[i])
		}
		if out := oklabToSRGB(got); out != ref.Conversion.Output[i] || out != c {
			t.Fatalf("color %d: round trip %v, old %v, input %v", i, out, ref.Conversion.Output[i], c)
		}
	}
	if len(ref.Clamp.Bits) != 4 || len(ref.Clamp.Output) != 4 {
		t.Fatal("clamp reference length mismatch")
	}
	for i, bits := range ref.Clamp.Bits {
		c := oklab{math.Float32frombits(bits[0]), math.Float32frombits(bits[1]), math.Float32frombits(bits[2])}
		if got := oklabToSRGB(c); got != ref.Clamp.Output[i] {
			t.Errorf("clamp %v: got %v, old %v", c, got, ref.Clamp.Output[i])
		}
	}
}

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
