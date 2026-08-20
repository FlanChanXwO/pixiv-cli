package browsercookies_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/browsercookies"
)

func TestSecretNeverLeaksInFormatting(t *testing.T) {
	const value = "super-secret-FANBOXSESSID-value"
	s := browsercookies.NewSecret(value)
	if s.Value() != value {
		t.Fatalf("Value() = %q, want %q", s.Value(), value)
	}
	formats := []string{"%v", "%s", "%+v", "%q", "%#v", "%x", "%X", "%20s", "% -s"}
	for _, f := range formats {
		got := fmt.Sprintf(f, s)
		if strings.Contains(got, value) {
			t.Fatalf("format %s leaked secret: %q", f, got)
		}
		if got == value {
			t.Fatalf("format %s returned raw secret", f)
		}
	}
	if got := fmt.Sprint(s); got != "<redacted>" {
		t.Fatalf("Sprint = %q, want %q", got, "<redacted>")
	}
	// 错误路径也不得泄露。
	err := fmt.Errorf("wrap: %v", s)
	if strings.Contains(err.Error(), value) {
		t.Fatalf("error leaked secret: %q", err.Error())
	}
}

func TestSecretNeverLeaksInJSON(t *testing.T) {
	const value = "json-secret-value"
	s := browsercookies.NewSecret(value)
	body, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), value) {
		t.Fatalf("json leaked secret: %s", body)
	}
	// 结构体字段序列化同样不得泄露。
	wrapper := struct {
		Session browsercookies.Secret `json:"session"`
	}{Session: s}
	body, err = json.Marshal(wrapper)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), value) {
		t.Fatalf("json wrapper leaked secret: %s", body)
	}
}

func TestCookieQueryValid(t *testing.T) {
	valid := []browsercookies.CookieQuery{
		{Host: ".fanbox.cc", Name: "FANBOXSESSID"},
		{Host: "fanbox.cc", Name: "a-b_c.d"},
	}
	for _, q := range valid {
		if err := q.Valid(); err != nil {
			t.Fatalf("Valid(%+v) = %v, want nil", q, err)
		}
	}
	if err := browsercookies.DefaultQuery.Valid(); err != nil {
		t.Fatalf("DefaultQuery invalid: %v", err)
	}
	invalid := []browsercookies.CookieQuery{
		{},
		{Host: "", Name: "X"},
		{Host: "x", Name: ""},
		{Host: "a b", Name: "X"},
		{Host: "a;DROP TABLE cookies", Name: "X"},
		{Host: "x", Name: "y\nz"},
		{Host: "x", Name: `FANBOXSESSID" OR 1=1`},
	}
	for _, q := range invalid {
		if err := q.Valid(); err == nil {
			t.Fatalf("Valid(%+v) = nil, want error", q)
		}
	}
}

func TestNewUnknownBrowser(t *testing.T) {
	for _, name := range []string{"netscape", "", "OPERA", "   "} {
		if _, err := browsercookies.New(name); !errors.Is(err, browsercookies.ErrUnknownBrowser) {
			t.Fatalf("New(%q) err = %v, want ErrUnknownBrowser", name, err)
		}
	}
}

var testProviderRegistered = false

// registerTestProvider 注册一个幂等的测试 provider。
func registerTestProvider(t *testing.T) {
	t.Helper()
	if testProviderRegistered {
		return
	}
	testProviderRegistered = true
	browsercookies.Register("test-browser", func() (browsercookies.Provider, error) {
		return testProvider{}, nil
	})
}

type testProvider struct{}

func (testProvider) Name() string { return "test-browser" }
func (testProvider) DiscoverProfiles(context.Context) ([]browsercookies.Profile, error) {
	return nil, nil
}
func (testProvider) Read(context.Context, browsercookies.CookieQuery, string) ([]browsercookies.Secret, error) {
	return nil, nil
}
func (testProvider) Close() error { return nil }

func TestNewDispatchRegisteredProvider(t *testing.T) {
	registerTestProvider(t)
	p, err := browsercookies.New("test-browser")
	if err != nil {
		t.Fatal(err)
	}
	if p.Name() != "test-browser" {
		t.Fatalf("Name() = %q", p.Name())
	}
	// 大小写不敏感。
	if _, err := browsercookies.New("TEST-BROWSER"); err != nil {
		t.Fatalf("New uppercase err = %v", err)
	}
}

func TestSelectProfile(t *testing.T) {
	profiles := []browsercookies.Profile{
		{ID: "Default", Name: "Default", Path: "/private/var/aaa"},
		{ID: "Profile 1", Name: "Profile 1", Path: "/private/var/bbb"},
	}
	// 多个 profile 且未指定 → ErrMultipleProfiles，错误只列出安全 identifier。
	_, err := browsercookies.SelectProfile(profiles, "")
	if !errors.Is(err, browsercookies.ErrMultipleProfiles) {
		t.Fatalf("err = %v, want ErrMultipleProfiles", err)
	}
	if strings.Contains(err.Error(), "/private") {
		t.Fatalf("multiple-profile error leaked a path: %q", err.Error())
	}
	// 单个 profile 自动选择。
	got, err := browsercookies.SelectProfile([]browsercookies.Profile{{ID: "Default", Path: "/x"}}, "")
	if err != nil || got.ID != "Default" {
		t.Fatalf("single profile select = %+v, %v", got, err)
	}
	// 指定 identifier。
	got, err = browsercookies.SelectProfile(profiles, "Profile 1")
	if err != nil || got.ID != "Profile 1" || got.Path != "/private/var/bbb" {
		t.Fatalf("select by id = %+v, %v", got, err)
	}
	// 未知 identifier。
	if _, err := browsercookies.SelectProfile(profiles, "nope"); !errors.Is(err, browsercookies.ErrProfileNotFound) {
		t.Fatalf("unknown id err = %v", err)
	}
	// 空列表。
	if _, err := browsercookies.SelectProfile(nil, ""); !errors.Is(err, browsercookies.ErrProfileNotFound) {
		t.Fatalf("empty list err = %v", err)
	}
}

func TestProviderFixtureHook(t *testing.T) {
	restore := browsercookies.SetProviderFixtureForTest("testbrowser", func(path string) ([]byte, error) {
		return []byte("fixture:" + path), nil
	})
	defer restore()
	data, hooked, err := browsercookies.HookBytes("testbrowser", "/some/path")
	if err != nil {
		t.Fatal(err)
	}
	if !hooked {
		t.Fatal("expected hooked=true")
	}
	if string(data) != "fixture:/some/path" {
		t.Fatalf("hook data = %q", data)
	}
	// 其他 provider 不命中。
	data, hooked, err = browsercookies.HookBytes("other", "/some/path")
	if err != nil || hooked || data != nil {
		t.Fatalf("other provider: hooked=%v data=%v err=%v", hooked, data, err)
	}
	// 恢复后不再命中。
	restore()
	if _, hooked, _ := browsercookies.HookBytes("testbrowser", "/some/path"); hooked {
		t.Fatal("expected no hook after restore")
	}
}

func TestWriteTempSnapshotPermissions(t *testing.T) {
	path, cleanup, err := browsercookies.WriteTempSnapshot([]byte("db-bytes"))
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("file mode = %o, want 600", got)
	}
	dirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if got := dirInfo.Mode().Perm(); got != 0o700 {
		t.Fatalf("dir mode = %o, want 700", got)
	}
	cleanup()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("cleanup did not remove snapshot (err=%v)", err)
	}
}
