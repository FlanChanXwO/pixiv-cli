package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/pixiv"
)

type syntheticAuthBinaryFixture struct {
	repoRoot      string
	binaryPath    string
	env           isolatedProcessEnv
	authPath      string
	defaultToken  string
	explicitToken string
	secrets       [][]byte
}

type syntheticAuthCommandResult struct {
	stdout []byte
	stderr []byte
	err    error
}

func newSyntheticAuthBinaryFixture(t *testing.T) syntheticAuthBinaryFixture {
	t.Helper()
	repoRoot := filepath.Join("..", "..")
	fixture := syntheticAuthBinaryFixture{
		repoRoot:      repoRoot,
		binaryPath:    buildPixivBinary(t, repoRoot),
		env:           isolatedEnv(t),
		defaultToken:  "synthetic-default-refresh-secret",
		explicitToken: "synthetic-explicit-refresh-secret",
	}
	fixture.secrets = [][]byte{[]byte(fixture.defaultToken), []byte(fixture.explicitToken)}
	fixture.authPath = fixture.resolveAuthPath(t, fixture.env)
	fixture.writeStore(t, fixture.authPath, 101, []syntheticAuthAccount{
		{UserID: 101, Username: "default-user", RefreshToken: fixture.defaultToken},
		{UserID: 202, Username: "explicit-user", RefreshToken: fixture.explicitToken},
	})
	return fixture
}

type syntheticAuthAccount struct {
	UserID       int64  `json:"user_id"`
	Username     string `json:"username"`
	RefreshToken string `json:"refresh_token"`
}

func (f syntheticAuthBinaryFixture) resolveAuthPath(t *testing.T, env isolatedProcessEnv) string {
	t.Helper()
	result := f.runWithEnv(t, env, nil, "config", "path")
	f.requireSuccess(t, result)
	f.requireEmpty(t, result.stderr, "config path stderr")
	configPath := filepath.Clean(string(bytes.TrimSpace(result.stdout)))
	if filepath.Base(configPath) != "config.toml" {
		t.Fatalf("config path did not end in config.toml: bytes=%d", len(result.stdout))
	}
	return filepath.Join(filepath.Dir(configPath), "auth.json")
}

func (f syntheticAuthBinaryFixture) writeStore(t *testing.T, authPath string, defaultUserID int64, accounts []syntheticAuthAccount) {
	t.Helper()
	body, err := json.Marshal(struct {
		DefaultUserID int64                  `json:"default_user_id"`
		Accounts      []syntheticAuthAccount `json:"accounts"`
	}{DefaultUserID: defaultUserID, Accounts: accounts})
	if err != nil {
		t.Fatalf("encode synthetic auth store: %v", err)
	}
	directory := filepath.Dir(authPath)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatalf("create synthetic auth directory: %v", err)
	}
	if err := os.WriteFile(authPath, body, 0o600); err != nil {
		t.Fatalf("write synthetic auth store: %v", err)
	}
}

func (f syntheticAuthBinaryFixture) run(t *testing.T, stdin []byte, args ...string) syntheticAuthCommandResult {
	t.Helper()
	return f.runWithEnv(t, f.env, stdin, args...)
}

func (f syntheticAuthBinaryFixture) runWithEnv(t *testing.T, env isolatedProcessEnv, stdin []byte, args ...string) syntheticAuthCommandResult {
	t.Helper()
	command := exec.CommandContext(testCommandContext(t), f.binaryPath, args...)
	command.Dir = f.repoRoot
	command.Env = env.values
	command.Stdin = bytes.NewReader(stdin)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	return syntheticAuthCommandResult{stdout: stdout.Bytes(), stderr: stderr.Bytes(), err: err}
}

func (f syntheticAuthBinaryFixture) newOfflineEnv(t *testing.T) isolatedProcessEnv {
	t.Helper()
	env := isolatedEnv(t)
	values := make([]string, 0, len(env.values)+3)
	for _, entry := range env.values {
		name, _, found := strings.Cut(entry, "=")
		if found && slicesContainsFold([]string{"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "NO_PROXY"}, name) {
			continue
		}
		values = append(values, entry)
	}
	// 若 restore 意外触发 HTTP，所有常见 proxy 入口都只会连接本机拒绝端口。
	values = append(values,
		"HTTP_PROXY=http://127.0.0.1:1",
		"HTTPS_PROXY=http://127.0.0.1:1",
		"ALL_PROXY=http://127.0.0.1:1",
	)
	env.values = values
	return env
}

func slicesContainsFold(values []string, target string) bool {
	for _, value := range values {
		if strings.EqualFold(value, target) {
			return true
		}
	}
	return false
}

func (f syntheticAuthBinaryFixture) requireSuccess(t *testing.T, result syntheticAuthCommandResult) {
	t.Helper()
	if result.err != nil {
		t.Fatalf("synthetic auth command failed: %s; stdout bytes=%d; stderr bytes=%d", canaryCommandFailureSummary(result.err), len(result.stdout), len(result.stderr))
	}
}

func (f syntheticAuthBinaryFixture) requireFailure(t *testing.T, result syntheticAuthCommandResult, operation string) {
	t.Helper()
	if result.err == nil {
		t.Fatalf("%s unexpectedly succeeded: stdout bytes=%d; stderr bytes=%d", operation, len(result.stdout), len(result.stderr))
	}
}

func (f syntheticAuthBinaryFixture) requireExactSecretOutput(t *testing.T, got, want []byte, operation string) {
	t.Helper()
	if !bytes.Equal(got, want) {
		t.Fatalf("%s output mismatch: got bytes=%d, want bytes=%d", operation, len(got), len(want))
	}
}

func (f syntheticAuthBinaryFixture) requireEmpty(t *testing.T, body []byte, stream string) {
	t.Helper()
	if len(body) != 0 {
		t.Fatalf("%s was not empty: bytes=%d", stream, len(body))
	}
}

func (f syntheticAuthBinaryFixture) requireNoSecrets(t *testing.T, body []byte, stream string) {
	t.Helper()
	for index, secret := range f.secrets {
		if bytes.Contains(body, secret) {
			t.Fatalf("%s contained synthetic secret index %d", stream, index)
		}
	}
}

func (f syntheticAuthBinaryFixture) requireSafeOutputSummary(t *testing.T, result syntheticAuthCommandResult, path string, accounts int) {
	t.Helper()
	f.requireNoSecrets(t, result.stdout, "output summary stdout")
	f.requireNoSecrets(t, result.stderr, "output summary stderr")
	f.requireEmpty(t, result.stderr, "output summary stderr")
	want := []byte(fmt.Sprintf("output: %s\naccounts: %d\n", path, accounts))
	if !bytes.Equal(result.stdout, want) {
		t.Fatalf("output summary mismatch: got bytes=%d; want bytes=%d", len(result.stdout), len(want))
	}
}

func (f syntheticAuthBinaryFixture) requireSingleBundleFile(t *testing.T, path string, userID int64, token string) {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read synthetic export bundle: %v", err)
	}
	bundle, err := pixiv.DecodeAuthExportBundle(body)
	if err != nil {
		t.Fatalf("decode synthetic export bundle: %v; body omitted", err)
	}
	if bundle.DefaultUserID != userID || len(bundle.Accounts) != 1 || bundle.Accounts[0].UserID != userID {
		t.Fatalf("single export bundle selection mismatch: default=%d accounts=%d", bundle.DefaultUserID, len(bundle.Accounts))
	}
	if !bytes.Equal([]byte(bundle.Accounts[0].RefreshToken), []byte(token)) {
		t.Fatal("single export bundle token mismatch")
	}
}

func (f syntheticAuthBinaryFixture) requireSafeRestoreReport(t *testing.T, result syntheticAuthCommandResult) {
	t.Helper()
	f.requireNoSecrets(t, result.stdout, "restore stdout")
	f.requireNoSecrets(t, result.stderr, "restore stderr")
	f.requireEmpty(t, result.stderr, "restore stderr")
	want := []byte("added uid:101\nadded uid:202\ndefault uid: 101\n")
	if !bytes.Equal(result.stdout, want) {
		t.Fatalf("restore report mismatch: got bytes=%d; want bytes=%d", len(result.stdout), len(want))
	}
}

func (f syntheticAuthBinaryFixture) requireRestoredAccountList(t *testing.T, env isolatedProcessEnv) {
	t.Helper()
	result := f.runWithEnv(t, env, nil, "auth", "list", "--json")
	f.requireSuccess(t, result)
	f.requireNoSecrets(t, result.stdout, "restored account list stdout")
	f.requireNoSecrets(t, result.stderr, "restored account list stderr")
	var list struct {
		DefaultUserID int64 `json:"default_user_id"`
		Accounts      []struct {
			UserID   int64 `json:"user_id"`
			Default  bool  `json:"default"`
			HasToken bool  `json:"has_token"`
		} `json:"accounts"`
	}
	if err := json.Unmarshal(result.stdout, &list); err != nil {
		t.Fatalf("decode restored account list: %v; body omitted", err)
	}
	if list.DefaultUserID != 101 || len(list.Accounts) != 2 {
		t.Fatalf("restored account list mismatch: default=%d accounts=%d", list.DefaultUserID, len(list.Accounts))
	}
	if list.Accounts[0].UserID != 101 || !list.Accounts[0].Default || !list.Accounts[0].HasToken {
		t.Fatal("restored default account summary mismatch")
	}
	if list.Accounts[1].UserID != 202 || list.Accounts[1].Default || !list.Accounts[1].HasToken {
		t.Fatal("restored explicit account summary mismatch")
	}
}

func (f syntheticAuthBinaryFixture) requireAllBundle(t *testing.T, body []byte) {
	t.Helper()
	if !bytes.HasSuffix(body, []byte("\n")) {
		t.Fatal("all export bundle did not end with LF")
	}
	bundle, err := pixiv.DecodeAuthExportBundle(body)
	if err != nil {
		t.Fatalf("decode all export bundle: %v; body omitted", err)
	}
	if bundle.Schema != pixiv.AuthExportBundleSchema || bundle.Version != pixiv.AuthExportBundleVersion {
		t.Fatal("all export bundle schema/version mismatch")
	}
	if bundle.DefaultUserID != 101 || len(bundle.Accounts) != 2 {
		t.Fatalf("all export bundle selection mismatch: default=%d accounts=%d", bundle.DefaultUserID, len(bundle.Accounts))
	}
	if bundle.Accounts[0].UserID != 101 || !bytes.Equal([]byte(bundle.Accounts[0].RefreshToken), []byte(f.defaultToken)) {
		t.Fatal("all export default account mismatch")
	}
	if bundle.Accounts[1].UserID != 202 || !bytes.Equal([]byte(bundle.Accounts[1].RefreshToken), []byte(f.explicitToken)) {
		t.Fatal("all export explicit account mismatch")
	}
}
