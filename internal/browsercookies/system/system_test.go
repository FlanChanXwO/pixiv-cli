package system_test

import (
	"errors"
	"strings"
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
	if _, err := system.New("unknown-browser"); !errors.Is(err, system.ErrUnknownBrowser) {
		t.Fatalf("New(unknown) err = %v, want ErrUnknownBrowser", err)
	}
	if _, err := system.New("chromium"); !errors.Is(err, system.ErrUnknownBrowser) {
		t.Fatalf("New(chromium) err = %v, want ErrUnknownBrowser", err)
	}
}

func TestDefaultQuery(t *testing.T) {
	if system.DefaultQuery.Host != ".fanbox.cc" || system.DefaultQuery.Name != "FANBOXSESSID" {
		t.Fatalf("DefaultQuery = %+v", system.DefaultQuery)
	}
	if err := system.DefaultQuery.Valid(); err != nil {
		t.Fatalf("DefaultQuery.Valid() = %v", err)
	}
}

func TestErrorIdentity(t *testing.T) {
	// re-export 错误与 root core 同一标识，errors.Is 可匹配。
	if !errors.Is(system.ErrUnknownBrowser, browsercookies.ErrUnknownBrowser) {
		t.Fatal("ErrUnknownBrowser identity broken")
	}
	if !errors.Is(system.ErrDatabaseLocked, browsercookies.ErrDatabaseLocked) {
		t.Fatal("ErrDatabaseLocked identity broken")
	}
}

func TestSelectProfileListsOnlyIDs(t *testing.T) {
	_, err := system.SelectProfile([]system.Profile{
		{ID: "Default", Path: "/secret/abs/path"},
		{ID: "Profile 1", Path: "/secret/abs/path2"},
	}, "")
	if !errors.Is(err, system.ErrMultipleProfiles) {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(err.Error(), "/secret") {
		t.Fatalf("error leaked a path: %q", err.Error())
	}
	if !strings.Contains(err.Error(), "Default") || !strings.Contains(err.Error(), "Profile 1") {
		t.Fatalf("error should list safe identifiers: %q", err.Error())
	}
}

func TestFixtureHookReexport(t *testing.T) {
	restore := system.SetProviderFixtureForTest("chrome", func(path string) ([]byte, error) {
		return []byte("fixture"), nil
	})
	defer restore()
	// 通过 system re-export 的 hook 生效，root 侧读取命中。
	if _, hooked, _ := browsercookies.HookBytes("chrome", "/x"); !hooked {
		t.Fatal("expected hooked=true")
	}
}

func TestSecretTypeAliasRedacts(t *testing.T) {
	s := system.Secret(browsercookies.NewSecret("top-secret"))
	if got := s.String(); got == "top-secret" {
		t.Fatal("Secret alias leaked value via String")
	}
}
