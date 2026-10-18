package ugoira

import "testing"

func TestKmeansRNGReference(t *testing.T) {
	ref := readKmeansReference(t)
	rng := newKmeansRNG()
	for i, want := range ref.Words {
		if got := rng.next(); got != want {
			t.Fatalf("word %d: %x, old %x", i, got, want)
		}
	}
	for _, tc := range ref.Uniform {
		rng := newKmeansRNG()
		for i, want := range tc.Indices {
			if got := rng.index(tc.N); got != want {
				t.Fatalf("range %d sample %d: %d, old %d", tc.N, i, got, want)
			}
		}
	}
}
