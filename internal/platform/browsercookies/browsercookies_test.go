package browsercookies

import (
	"errors"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/platform/browsercookies/core"
)

func TestNewDispatch(t *testing.T) {
	cases := []struct{ name, want string }{
		{"chrome", "chrome"},
		{"chromium", "chrome"},
		{"CHROME", "chrome"},
		{"edge", "edge"},
		{"firefox", "firefox"},
		{"safari", "safari"},
	}
	for _, c := range cases {
		p, err := New(c.name)
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
	if _, err := New("unknown-browser"); !errors.Is(err, ErrUnknownBrowser) {
		t.Fatalf("New(unknown) err = %v, want ErrUnknownBrowser", err)
	}
}

func TestDefaultQuery(t *testing.T) {
	if DefaultQuery.Host != ".fanbox.cc" || DefaultQuery.Name != "FANBOXSESSID" {
		t.Fatalf("DefaultQuery = %+v", DefaultQuery)
	}
	if err := DefaultQuery.Valid(); err != nil {
		t.Fatalf("DefaultQuery.Valid() = %v", err)
	}
}

func TestErrorIdentity(t *testing.T) {
	// 重导出错误与 core 同一标识，errors.Is 可匹配。
	if !errors.Is(ErrUnknownBrowser, ErrUnknownBrowser) {
		t.Fatal("ErrUnknownBrowser identity broken")
	}
	if !errors.Is(ErrDatabaseLocked, ErrDatabaseLocked) {
		t.Fatal("ErrDatabaseLocked identity broken")
	}
}

func TestSelectProfileListsOnlyIDs(t *testing.T) {
	_, err := SelectProfile([]Profile{
		{ID: "Default", Path: "/secret/abs/path"},
		{ID: "Profile 1", Path: "/secret/abs/path2"},
	}, "")
	if !errors.Is(err, ErrMultipleProfiles) {
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
	restore := SetProviderFixtureForTest("chrome", func(path string) ([]byte, error) {
		return []byte("fixture"), nil
	})
	defer restore()
	// 通过父包 re-export 的 hook 生效，core 侧读取命中。
	if _, hooked, _ := core.HookBytes("chrome", "/x"); !hooked {
		t.Fatal("expected hooked=true")
	}
}

func TestSecretTypeAliasRedacts(t *testing.T) {
	s := Secret(core.NewSecret("top-secret"))
	if got := s.String(); got == "top-secret" {
		t.Fatal("Secret alias leaked value via String")
	}
}
