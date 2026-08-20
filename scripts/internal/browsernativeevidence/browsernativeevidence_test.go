package browsernativeevidence

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestValidateCheckedInWorkflow(t *testing.T) {
	if err := Validate(filepath.Join(findRepositoryRoot(t), ".github", "workflows", "browser-evidence.yml")); err != nil {
		t.Fatal(err)
	}
}

func TestValidateRejectsCredentialAndUnpinnedActionMutations(t *testing.T) {
	root := findRepositoryRoot(t)
	body, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "browser-evidence.yml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, mutation := range []string{
		string(body) + "\n      - run: echo ${{ secrets.FANBOX_SESSION }}\n",
		strings.Replace(string(body), checkoutAction, "actions/checkout@main", 1),
	} {
		path := filepath.Join(t.TempDir(), "browser-evidence.yml")
		if err := os.WriteFile(path, []byte(mutation), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := Validate(path); err == nil {
			t.Fatalf("Validate accepted unsafe workflow mutation in %s", path)
		}
	}
}

func TestValidateRejectsFirefoxChecksumArchitectureSwap(t *testing.T) {
	root := findRepositoryRoot(t)
	body, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "browser-evidence.yml"))
	if err != nil {
		t.Fatal(err)
	}
	linuxAMD64 := "22b312280900bfb174b685ece32c7b3c6d72e7f8e53d6d30f21ac41a8dc500a2"
	linuxARM64 := "c19b325accedebbc3a1235e3c7104d80c5a4412b368f7d0935b4718114416870"
	mutated := strings.Replace(string(body), linuxAMD64, "checksum-swap-placeholder", 1)
	mutated = strings.Replace(mutated, linuxARM64, linuxAMD64, 1)
	mutated = strings.Replace(mutated, "checksum-swap-placeholder", linuxARM64, 1)
	path := filepath.Join(t.TempDir(), "browser-evidence.yml")
	if err := os.WriteFile(path, []byte(mutated), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Validate(path); err == nil {
		t.Fatal("Validate accepted a Firefox architecture/checksum swap")
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
