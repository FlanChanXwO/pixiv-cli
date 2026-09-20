package browsernativeevidence

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

func TestPolicySubcommandIsNoLongerPublic(t *testing.T) {
	t.Parallel()

	if got := Run([]string{"policy", "--workflow", ".github/workflows/browser-evidence.yml"}); got != 2 {
		t.Fatalf("Run(policy) exit = %d, want usage exit 2", got)
	}
}

func TestBrowserEvidenceWorkflowKeepsSecurityAndFixtureBoundaries(t *testing.T) {
	t.Parallel()

	root := findRepositoryRoot(t)
	body, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "browser-evidence.yml"))
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(body)
	for _, required := range []string{
		"workflow_dispatch: {}",
		"permissions: {}",
		"browser_provider:",
		"firefox_native:",
		"firefox_url:",
		"firefox_sha256:",
		"go run ./scripts/cmd/browsernativeevidence firefox-contract --firefox",
		"go test ./internal/browsercookies/... -count=1 -v",
		"go test ./e2e -run '^TestNativeBrowserNamesRejectInvalidInput$' -count=1",
		"Remove Firefox package and installation on Unix",
		"Remove Firefox package and installation on Windows",
	} {
		if !strings.Contains(workflow, required) {
			t.Fatalf("browser evidence workflow missing contract %q", required)
		}
	}
	for _, forbidden := range []string{
		"secrets.",
		"environment:",
		"FANBOXSESSID",
		"BROWSER_NATIVE_E2E=1",
		"security find-generic-password",
		"--from-browser",
		"actions/upload-artifact@",
	} {
		if strings.Contains(workflow, forbidden) {
			t.Fatalf("browser evidence workflow contains forbidden boundary %q", forbidden)
		}
	}
	writePermission := regexp.MustCompile("(?m)^\\s+[A-Za-z-]+:\\s*write\\s*$")
	if writePermission.MatchString(workflow) {
		t.Fatal("browser evidence workflow must not grant write permissions")
	}
	pinnedAction := regexp.MustCompile("^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+@[0-9a-f]{40}$")
	for line := range strings.SplitSeq(workflow, "\n") {
		trimmed := strings.TrimSpace(line)
		trimmed = strings.TrimPrefix(trimmed, "- ")
		if !strings.HasPrefix(trimmed, "uses: actions/") {
			continue
		}
		if !pinnedAction.MatchString(strings.TrimPrefix(trimmed, "uses: ")) {
			t.Fatalf("GitHub action must be pinned to a full commit SHA: %s", trimmed)
		}
	}
}
func TestFirefoxEvidenceHelpersUseIsolatedPathsAndEnvironment(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	root, err := firefoxDataRootFor(home)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"darwin":  filepath.Join(home, "Library", "Application Support", "Firefox"),
		"linux":   filepath.Join(home, ".config", "mozilla", "firefox"),
		"windows": filepath.Join(home, "AppData", "Roaming", "Mozilla", "Firefox"),
	}
	if root != want[runtime.GOOS] {
		t.Fatalf("Firefox data root = %q, want %q", root, want[runtime.GOOS])
	}

	env := isolatedFirefoxEnvironment([]string{"HOME=old", "XDG_CONFIG_HOME=old", "KEEP=1"}, home)
	for _, key := range []string{"HOME", "XDG_CONFIG_HOME", "USERPROFILE", "APPDATA", "LOCALAPPDATA", "MOZ_HEADLESS"} {
		matches := 0
		for _, entry := range env {
			if strings.HasPrefix(entry, key+"=") {
				matches++
				if key == "HOME" && entry != "HOME="+home {
					t.Fatalf("HOME environment = %q", entry)
				}
			}
		}
		if matches != 1 {
			t.Fatalf("environment contains %d %s entries, want one", matches, key)
		}
	}
}

func TestValidateFirefoxExecutablePath(t *testing.T) {
	if err := validateFirefoxExecutablePath(filepath.Join("relative", "firefox")); err == nil {
		t.Fatal("relative Firefox executable path unexpectedly accepted")
	}
	path := filepath.Join(t.TempDir(), "firefox")
	if err := os.WriteFile(path, []byte("synthetic executable placeholder"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := validateFirefoxExecutablePath(path); err != nil {
		t.Fatalf("regular Firefox executable path rejected: %v", err)
	}
}

func TestCurrentGoEnvironmentIsPreservedAcrossHomeIsolation(t *testing.T) {
	values, err := currentGoEnvironment(os.Environ())
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"GOPATH", "GOMODCACHE", "GOCACHE"} {
		value := values[key]
		if value == "" {
			t.Fatalf("current Go environment has empty %s", key)
		}
		isolated := setEnvironment([]string{"HOME=/temporary-home"}, key, value)
		found := false
		for _, entry := range isolated {
			if entry == key+"="+value {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("isolated environment did not preserve %s", key)
		}
	}
}

func TestSeedSyntheticFirefoxCookieUsesExpectedSchema(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 command-line tool not available")
	}
	databasePath := filepath.Join(t.TempDir(), "cookies.sqlite")
	create := `CREATE TABLE moz_cookies (
name TEXT, value TEXT, host TEXT, path TEXT, expiry INTEGER, lastAccessed INTEGER,
creationTime INTEGER, isSecure INTEGER, isHttpOnly INTEGER, inBrowserElement INTEGER,
sameSite INTEGER, rawSameSite INTEGER, schemeMap INTEGER, originAttributes TEXT,
UNIQUE(name, host, path, originAttributes)
);`
	command := exec.Command("sqlite3", databasePath, create)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("create SQLite schema: %v: %s", err, output)
	}
	if err := seedSyntheticFirefoxCookie(databasePath); err != nil {
		t.Fatal(err)
	}
	query := exec.Command("sqlite3", databasePath, "SELECT name || '|' || value || '|' || host FROM moz_cookies;")
	output, err := query.Output()
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(output)); got != "FANBOXSESSID|browser-native-evidence-synthetic|.fanbox.cc" {
		t.Fatalf("synthetic cookie row = %q", got)
	}
}

func findRepositoryRoot(t *testing.T) string {
	t.Helper()
	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(root, "go.mod")); err == nil {
			return root
		}
		parent := filepath.Dir(root)
		if parent == root {
			t.Fatal("could not find repository root")
		}
		root = parent
	}
}
