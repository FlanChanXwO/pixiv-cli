package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAccountAddListUseRemove(t *testing.T) {
	path := testConfigPath(t)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "account", "add", "--token", "foo=bar; refresh_token=main%2Ftoken", "main"}, strings.NewReader(""), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("add main code=%d stderr=%s", code, stderr.String())
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat config: %v", err)
	}
	if got := info.Mode().Perm(); got != defaultConfigFileMode {
		t.Fatalf("config mode = %v", got)
	}
	store, err := loadAccountStore(path)
	if err != nil {
		t.Fatalf("load store: %v", err)
	}
	if store.DefaultProfile != "main" || store.Accounts["main"].RefreshToken != "main/token" {
		t.Fatalf("store = %+v", store)
	}

	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"pixiv", "account", "add", "--token", "other-token", "other"}, strings.NewReader(""), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("add other code=%d stderr=%s", code, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"pixiv", "account", "use", "other"}, strings.NewReader(""), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("use code=%d stderr=%s", code, stderr.String())
	}
	store, err = loadAccountStore(path)
	if err != nil {
		t.Fatalf("load store: %v", err)
	}
	if store.DefaultProfile != "other" {
		t.Fatalf("default profile = %q", store.DefaultProfile)
	}

	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"pixiv", "account", "list", "--json"}, strings.NewReader(""), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("list code=%d stderr=%s", code, stderr.String())
	}
	var out accountListOut
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("decode JSON: %v\n%s", err, stdout.String())
	}
	if out.DefaultProfile != "other" || len(out.Accounts) != 2 {
		t.Fatalf("list output = %+v", out)
	}
	if strings.Contains(stdout.String(), "other-token") || strings.Contains(stdout.String(), "main/token") {
		t.Fatalf("list leaked token: %s", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"pixiv", "account", "remove", "other"}, strings.NewReader(""), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("remove code=%d stderr=%s", code, stderr.String())
	}
	store, err = loadAccountStore(path)
	if err != nil {
		t.Fatalf("load store: %v", err)
	}
	if store.DefaultProfile != "main" {
		t.Fatalf("default after remove = %q", store.DefaultProfile)
	}
}

func TestAccountAddReadsTokenFromStdin(t *testing.T) {
	path := testConfigPath(t)
	var stdout, stderr bytes.Buffer

	code := Run([]string{"pixiv", "account", "add", "main"}, strings.NewReader("stdin-token\n"), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, stderr.String())
	}
	store, err := loadAccountStore(path)
	if err != nil {
		t.Fatalf("load store: %v", err)
	}
	if store.Accounts["main"].RefreshToken != "stdin-token" {
		t.Fatalf("store = %+v", store)
	}
}

func TestAccountAddRejectsCookieWithoutRefreshToken(t *testing.T) {
	testConfigPath(t)
	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "account", "add", "--token", "PHPSESSID=web; device_token=device", "main"}, strings.NewReader(""), &stdout, &stderr)
	if code == 0 {
		t.Fatalf("expected non-zero code")
	}
	if !strings.Contains(stderr.String(), "refresh_token") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestClientConfigProfilePriority(t *testing.T) {
	path := testConfigPath(t)
	if err := saveAccountStore(path, accountStore{
		DefaultProfile: "main",
		Accounts: map[string]account{
			"main":  {RefreshToken: "main-token"},
			"other": {RefreshToken: "other-token"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PIXIV_REFRESH_TOKEN", "env-token")

	a := app{in: strings.NewReader(""), out: &bytes.Buffer{}, errOut: &bytes.Buffer{}}
	client, _, err := a.clientAndConfig(commandOptions{}, false)
	if err != nil {
		t.Fatal(err)
	}
	if client.RefreshTokenValue() != "env-token" {
		t.Fatalf("token = %q", client.RefreshTokenValue())
	}
	client, _, err = a.clientAndConfig(commandOptions{profile: "other"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if client.RefreshTokenValue() != "other-token" {
		t.Fatalf("token = %q", client.RefreshTokenValue())
	}
	client, _, err = a.clientAndConfig(commandOptions{profile: "other", refreshToken: "flag-token"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if client.RefreshTokenValue() != "flag-token" {
		t.Fatalf("token = %q", client.RefreshTokenValue())
	}
}

func testConfigPath(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "pixiv", "config.json")
	old := configPath
	configPath = func() (string, error) { return path, nil }
	t.Cleanup(func() { configPath = old })
	return path
}
