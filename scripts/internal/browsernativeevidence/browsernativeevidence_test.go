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
		"Remove Firefox package and installation on Unix",
		"Remove Firefox package and installation on Windows",
	} {
		if !strings.Contains(workflow, required) {
			t.Fatalf("browser evidence workflow missing contract %q", required)
		}
	}
	for _, forbidden := range []string{
		"\n  push:",
		"secrets.",
		"environment:",
		"FANBOXSESSID",
		"BROWSER_NATIVE_E2E=1",
		"security find-generic-password",
		"--from-browser",
		"actions/upload-artifact@",
		"go test ./e2e",
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
		if !strings.HasPrefix(trimmed, "uses: ") {
			continue
		}
		reference := strings.TrimPrefix(trimmed, "uses: ")
		if strings.HasPrefix(reference, "./") || strings.HasPrefix(reference, "$/") {
			continue
		}
		if !pinnedAction.MatchString(reference) {
			t.Fatalf("GitHub action must be pinned to a full commit SHA: %s", trimmed)
		}
	}
}

func TestWindowsBrowserEvidencePinsSQLiteCLIProvisioning(t *testing.T) {
	t.Parallel()

	root := findRepositoryRoot(t)
	workflowBody, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "browser-evidence.yml"))
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(workflowBody)
	const installCommand = "./scripts/install-browser-sqlite.ps1 -GoArch '${{ matrix.goarch }}'"
	if got := strings.Count(workflow, installCommand); got != 2 {
		t.Fatalf("Windows SQLite provisioning calls = %d, want 2", got)
	}

	scriptBody, err := os.ReadFile(filepath.Join(root, "scripts", "install-browser-sqlite.ps1"))
	if err != nil {
		t.Fatal(err)
	}
	script := string(scriptBody)
	for _, required := range []string{
		"https://www.sqlite.org/2026/sqlite-tools-win-x64-3530400.zip",
		"https://www.sqlite.org/2026/sqlite-tools-win-arm64-3530400.zip",
		"f46ee2475de4cbe287e6e5f7d43c838796b14e7379cd216bdbb28d391429f9fc",
		"8a7c30165f6e9b054fbbe5ba6048acf23c967fd76955f7a5d66dc519542d3393",
		"Get-FileHash -Algorithm SHA256",
		"$env:GITHUB_PATH",
	} {
		if !strings.Contains(script, required) {
			t.Fatalf("Windows SQLite provisioning script missing %q", required)
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

func TestVerifySyntheticFirefoxProviderContractReadsIsolatedProfile(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 command-line tool not available")
	}
	originalEnvironment := map[string]string{
		"HOME":            "before-home",
		"XDG_CONFIG_HOME": "before-xdg",
		"USERPROFILE":     "before-userprofile",
		"APPDATA":         "before-appdata",
		"LOCALAPPDATA":    "before-localappdata",
	}
	for key, value := range originalEnvironment {
		t.Setenv(key, value)
	}
	home := filepath.Join(t.TempDir(), "home")
	dataRoot, err := firefoxDataRootFor(home)
	if err != nil {
		t.Fatal(err)
	}
	profileID := "ci.default-release"
	profileDir := filepath.Join(dataRoot, profileID)
	if err := os.MkdirAll(profileDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := writeFirefoxProfilesINI(dataRoot, profileID); err != nil {
		t.Fatal(err)
	}
	databasePath := filepath.Join(profileDir, "cookies.sqlite")
	create := exec.Command("sqlite3", databasePath, `CREATE TABLE moz_cookies (
name TEXT, value TEXT, host TEXT, path TEXT,
UNIQUE(name, host, path)
);`)
	if output, err := create.CombinedOutput(); err != nil {
		t.Fatalf("create Firefox fixture schema: %v: %s", err, output)
	}
	if err := seedSyntheticFirefoxCookie(databasePath); err != nil {
		t.Fatal(err)
	}

	if err := verifySyntheticFirefoxProviderContract(home, profileID); err != nil {
		t.Fatalf("verify synthetic Firefox provider contract: %v", err)
	}
	for key, want := range originalEnvironment {
		if got := os.Getenv(key); got != want {
			t.Fatalf("%s after provider contract = %q, want %q", key, got, want)
		}
	}
}

func TestVerifySyntheticFirefoxProviderContractRejectsWrongCookieValue(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 command-line tool not available")
	}
	home := filepath.Join(t.TempDir(), "home")
	dataRoot, err := firefoxDataRootFor(home)
	if err != nil {
		t.Fatal(err)
	}
	profileID := "ci.default-release"
	profileDir := filepath.Join(dataRoot, profileID)
	if err := os.MkdirAll(profileDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := writeFirefoxProfilesINI(dataRoot, profileID); err != nil {
		t.Fatal(err)
	}
	databasePath := filepath.Join(profileDir, "cookies.sqlite")
	statement := `CREATE TABLE moz_cookies (
name TEXT, value TEXT, host TEXT, path TEXT,
UNIQUE(name, host, path)
);
INSERT INTO moz_cookies (name, value, host, path)
VALUES ('FANBOXSESSID', 'wrong-but-nonempty', '.fanbox.cc', '/');`
	create := exec.Command("sqlite3", databasePath, statement)
	if output, err := create.CombinedOutput(); err != nil {
		t.Fatalf("create Firefox fixture with wrong cookie: %v: %s", err, output)
	}

	if err := verifySyntheticFirefoxProviderContract(home, profileID); err == nil {
		t.Fatal("wrong synthetic Firefox cookie value unexpectedly accepted")
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
