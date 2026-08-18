package firefox

import (
	"context"
	"database/sql"
	"errors"

	"github.com/FlanChanXwO/pixiv-cli/internal/browsercookies"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	_ "modernc.org/sqlite"
)

func skipIfNoSQLite3(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 command-line tool not available")
	}
}

const mozSchema = `CREATE TABLE moz_cookies (
	id INTEGER PRIMARY KEY,
	name TEXT,
	value TEXT,
	host TEXT,
	path TEXT
);`

func buildMozFixture(t *testing.T, statements ...string) []byte {
	t.Helper()
	path := filepath.Join(t.TempDir(), "cookies.sqlite")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	for _, stmt := range append([]string{mozSchema}, statements...) {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("exec %q: %v", stmt, err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func writeProfiles(t *testing.T, root string, ini string, dirs ...string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, profilesIni), []byte(ini), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, dir, cookiesDB), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDiscoverProfilesFromProfilesIni(t *testing.T) {
	root := t.TempDir()
	writeProfiles(t, root,
		"[Profile0]\nName=default\nIsRelative=1\nPath=default-release\n\n[Profile1]\nName=dev\nPath=devprofile\n",
		"default-release", "devprofile")

	p, err := newProvider(root)
	if err != nil {
		t.Fatal(err)
	}
	profiles, err := p.DiscoverProfiles(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 2 {
		t.Fatalf("profiles = %+v", profiles)
	}
	byID := map[string]browsercookies.Profile{}
	for _, pr := range profiles {
		byID[pr.ID] = pr
	}
	if pr, ok := byID["default-release"]; !ok || pr.Name != "default" {
		t.Fatalf("default-release profile = %+v", pr)
	}
	if pr, ok := byID["devprofile"]; !ok || pr.Name != "dev" {
		t.Fatalf("devprofile profile = %+v", pr)
	}
}

func TestDiscoverNotInstalled(t *testing.T) {
	p, _ := newProvider(filepath.Join(t.TempDir(), "missing"))
	if _, err := p.DiscoverProfiles(context.Background()); !errors.Is(err, browsercookies.ErrNotInstalled) {
		t.Fatalf("err = %v, want ErrNotInstalled", err)
	}

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, profilesIni), []byte("[Profile0]\nPath=ghost\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	p2, _ := newProvider(root)
	if _, err := p2.DiscoverProfiles(context.Background()); !errors.Is(err, browsercookies.ErrNotInstalled) {
		t.Fatalf("empty profiles err = %v, want ErrNotInstalled", err)
	}
}

func TestReadPlaintextCookie(t *testing.T) {
	skipIfNoSQLite3(t)
	root := t.TempDir()
	writeProfiles(t, root, "[Profile0]\nName=default\nPath=default-release\n", "default-release")

	fixture := buildMozFixture(t,
		`INSERT INTO moz_cookies (name, value, host, path) VALUES ('FANBOXSESSID', 'ff-session', '.fanbox.cc', '/');`,
	)
	restore := browsercookies.SetProviderFixtureForTest("firefox", func(string) ([]byte, error) { return fixture, nil })
	defer restore()

	p, _ := newProvider(root)
	secrets, err := p.Read(context.Background(), browsercookies.DefaultQuery, "default-release")
	if err != nil {
		t.Fatal(err)
	}
	if len(secrets) != 1 || secrets[0].Value() != "ff-session" {
		t.Fatalf("secrets = %+v", secrets)
	}
}

func TestReadEncryptedClassified(t *testing.T) {
	skipIfNoSQLite3(t)
	root := t.TempDir()
	writeProfiles(t, root, "[Profile0]\nName=default\nPath=default-release\n", "default-release")

	fixture := buildMozFixture(t,
		`INSERT INTO moz_cookies (name, value, host, path) VALUES ('FANBOXSESSID', X'8F8F8F8F8F', '.fanbox.cc', '/');`,
	)
	restore := browsercookies.SetProviderFixtureForTest("firefox", func(string) ([]byte, error) { return fixture, nil })
	defer restore()

	p, _ := newProvider(root)
	if _, err := p.Read(context.Background(), browsercookies.DefaultQuery, "default-release"); !errors.Is(err, browsercookies.ErrEncryptedCookieUnsupported) {
		t.Fatalf("err = %v, want ErrEncryptedCookieUnsupported", err)
	}
}

func TestReadInvalidInputs(t *testing.T) {
	root := t.TempDir()
	writeProfiles(t, root, "[Profile0]\nName=default\nPath=default-release\n", "default-release")
	p, _ := newProvider(root)

	if _, err := p.Read(context.Background(), browsercookies.CookieQuery{}, "default-release"); !errors.Is(err, browsercookies.ErrQueryInvalid) {
		t.Fatalf("invalid query err = %v", err)
	}
	if _, err := p.Read(context.Background(), browsercookies.DefaultQuery, "../x"); !errors.Is(err, browsercookies.ErrInvalidProfileID) {
		t.Fatalf("invalid profile err = %v", err)
	}
	if _, err := p.Read(context.Background(), browsercookies.DefaultQuery, "unknown-profile"); !errors.Is(err, browsercookies.ErrProfileNotFound) {
		t.Fatalf("unknown profile err = %v", err)
	}
}
func TestFirefoxDataRootMatchesSupportedPlatformLayout(t *testing.T) {
	home := filepath.FromSlash("/fixture-home")
	switch runtime.GOOS {
	case "darwin":
		if got := firefoxDataRoot(home); got != filepath.Join(home, "Library", "Application Support", "Firefox") {
			t.Fatalf("Firefox root = %q", got)
		}
	case "linux":
		configHome := filepath.Join(home, "xdg-config")
		t.Setenv("XDG_CONFIG_HOME", configHome)
		if got := firefoxDataRoot(home); got != filepath.Join(configHome, "mozilla", "firefox") {
			t.Fatalf("Firefox root = %q", got)
		}
	case "windows":
		if got := firefoxDataRoot(home); got != filepath.Join(home, "AppData", "Roaming", "Mozilla", "Firefox") {
			t.Fatalf("Firefox root = %q", got)
		}
	default:
		if got := firefoxDataRoot(home); got != "" {
			t.Fatalf("unsupported-platform Firefox root = %q, want empty", got)
		}
	}
}
