package system_test

import (
	"errors"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/browsercookies"
	"github.com/FlanChanXwO/pixiv-cli/internal/browsercookies/system"
)

func TestNewDispatch(t *testing.T) {
	cases := []struct{ name, want string }{
		{"chrome", "chrome"},
		{"CHROME", "chrome"},
		{"edge", "edge"},
		{"firefox", "firefox"},
		{"safari", "safari"},
	}
	for _, c := range cases {
		p, err := system.New(c.name)
		if err != nil {
			t.Fatalf("New(%q) err = %v", c.name, err)
		}
		if p.Name() != c.want {
			t.Fatalf("New(%q).Name() = %q, want %q", c.name, p.Name(), c.want)
		}
		if err := p.Close(); err != nil {
			t.Fatalf("Close() err = %v", err)
		}
	}
	if _, err := system.New("unknown-browser"); !errors.Is(err, browsercookies.ErrUnknownBrowser) {
		t.Fatalf("New(unknown) err = %v, want ErrUnknownBrowser", err)
	}
	if _, err := system.New("chromium"); !errors.Is(err, browsercookies.ErrUnknownBrowser) {
		t.Fatalf("New(chromium) err = %v, want ErrUnknownBrowser", err)
	}
}
