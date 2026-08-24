package assembly

import "testing"

func TestNewRejectsInvalidProxy(t *testing.T) {
	if _, err := New(Options{Proxy: "not a proxy"}); err == nil {
		t.Fatal("accepted an invalid reverse-search proxy")
	}
}
