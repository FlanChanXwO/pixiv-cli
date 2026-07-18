package e2e

import (
	"bufio"
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

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

type syntheticOAuthProxy struct {
	server       *httptest.Server
	caPath       string
	certificate  tls.Certificate
	rotatedToken string
	accessToken  string
	userID       int64
	mu           sync.Mutex
	received     [][]byte
	errors       []string
}

func newSyntheticOAuthProxy(t *testing.T, rotatedToken, accessToken string, userID int64) *syntheticOAuthProxy {
	t.Helper()
	certificate, caPEM := newSyntheticOAuthCertificate(t)
	caPath := filepath.Join(t.TempDir(), "synthetic-oauth-ca.pem")
	if err := os.WriteFile(caPath, caPEM, 0o600); err != nil {
		t.Fatalf("write synthetic OAuth CA: %v", err)
	}
	proxy := &syntheticOAuthProxy{
		caPath:       caPath,
		certificate:  certificate,
		rotatedToken: rotatedToken,
		accessToken:  accessToken,
		userID:       userID,
	}
	proxy.server = httptest.NewServer(http.HandlerFunc(proxy.serveHTTP))
	t.Cleanup(proxy.server.Close)
	return proxy
}

func newSyntheticOAuthCertificate(t *testing.T) (tls.Certificate, []byte) {
	t.Helper()
	now := time.Now()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate synthetic OAuth CA key: %v", err)
	}
	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "pixiv-cli synthetic OAuth test CA"},
		NotBefore:             now.Add(-time.Minute),
		NotAfter:              now.Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create synthetic OAuth CA certificate: %v", err)
	}
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate synthetic OAuth server key: %v", err)
	}
	leafTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "oauth.secure.pixiv.net"},
		DNSNames:     []string{"oauth.secure.pixiv.net"},
		NotBefore:    now.Add(-time.Minute),
		NotAfter:     now.Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTemplate, caTemplate, &leafKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create synthetic OAuth server certificate: %v", err)
	}
	return tls.Certificate{Certificate: [][]byte{leafDER, caDER}, PrivateKey: leafKey}, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})
}

func (p *syntheticOAuthProxy) serveHTTP(w http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodConnect || request.Host != "oauth.secure.pixiv.net:443" {
		p.recordError("proxy received an unexpected CONNECT target")
		http.Error(w, "synthetic OAuth proxy rejected request", http.StatusBadGateway)
		return
	}
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		p.recordError("proxy response writer cannot hijack")
		http.Error(w, "synthetic OAuth proxy cannot connect", http.StatusInternalServerError)
		return
	}
	connection, buffered, err := hijacker.Hijack()
	if err != nil {
		p.recordError("proxy CONNECT hijack failed")
		return
	}
	defer connection.Close()
	if _, err := buffered.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
		p.recordError("proxy CONNECT response write failed")
		return
	}
	if err := buffered.Flush(); err != nil {
		p.recordError("proxy CONNECT response flush failed")
		return
	}

	tlsConnection := tls.Server(connection, &tls.Config{
		Certificates: []tls.Certificate{p.certificate},
		MinVersion:   tls.VersionTLS12,
	})
	if err := tlsConnection.Handshake(); err != nil {
		p.recordError("synthetic OAuth TLS handshake failed")
		return
	}
	oauthRequest, err := http.ReadRequest(bufio.NewReader(tlsConnection))
	if err != nil {
		p.recordError("synthetic OAuth request read failed")
		return
	}
	defer oauthRequest.Body.Close()
	if oauthRequest.Method != http.MethodPost || oauthRequest.URL.Path != "/auth/token" || oauthRequest.Host != "oauth.secure.pixiv.net" {
		p.recordError("synthetic OAuth request target mismatch")
		return
	}
	if err := oauthRequest.ParseForm(); err != nil {
		p.recordError("synthetic OAuth form decode failed")
		return
	}
	p.mu.Lock()
	p.received = append(p.received, []byte(oauthRequest.Form.Get("refresh_token")))
	p.mu.Unlock()
	body, err := json.Marshal(map[string]any{
		"access_token":  p.accessToken,
		"refresh_token": p.rotatedToken,
		"user": map[string]any{
			"id": p.userID, "name": "synthetic-import-user",
		},
	})
	if err != nil {
		p.recordError("synthetic OAuth response encode failed")
		return
	}
	if _, err := fmt.Fprintf(tlsConnection, "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: %d\r\nConnection: close\r\n\r\n", len(body)); err != nil {
		p.recordError("synthetic OAuth response header write failed")
		return
	}
	if _, err := tlsConnection.Write(body); err != nil {
		p.recordError("synthetic OAuth response body write failed")
	}
}

func (p *syntheticOAuthProxy) recordError(message string) {
	p.mu.Lock()
	p.errors = append(p.errors, message)
	p.mu.Unlock()
}

func (p *syntheticOAuthProxy) proxyURL() string { return p.server.URL }

func (p *syntheticOAuthProxy) trustedEnv(env isolatedProcessEnv) isolatedProcessEnv {
	values := make([]string, 0, len(env.values)+1)
	for _, entry := range env.values {
		name, _, found := strings.Cut(entry, "=")
		if found && strings.EqualFold(name, "SSL_CERT_FILE") {
			continue
		}
		values = append(values, entry)
	}
	values = append(values, "SSL_CERT_FILE="+p.caPath)
	env.values = values
	return env
}

func (p *syntheticOAuthProxy) requireReceivedToken(t *testing.T, want string, count int) {
	t.Helper()
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.errors) != 0 {
		t.Fatalf("synthetic OAuth proxy errors: count=%d first=%q", len(p.errors), p.errors[0])
	}
	if len(p.received) != count {
		t.Fatalf("synthetic OAuth request count=%d, want %d", len(p.received), count)
	}
	for index, got := range p.received {
		if !bytes.Equal(got, []byte(want)) {
			t.Fatalf("synthetic OAuth refresh token mismatch at secret index %d: bytes=%d", index, len(got))
		}
	}
}

func newSyntheticAuthBinaryFixture(t *testing.T) syntheticAuthBinaryFixture {
	t.Helper()
	repoRoot := filepath.Join("..", "..")
	fixture := syntheticAuthBinaryFixture{
		repoRoot:      repoRoot,
		binaryPath:    buildPixivBinary(t, repoRoot),
		env:           newSyntheticAuthProcessEnv(t),
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

// newDirectImportFixture 复用最终 binary，但为 direct OAuth 子测分配独立的
// HOME/XDG/auth store。这样 UID 303 的 added/updated 状态不依赖 transfer fixture。
func (f syntheticAuthBinaryFixture) newDirectImportFixture(t *testing.T, secrets ...string) syntheticAuthBinaryFixture {
	t.Helper()
	fixture := syntheticAuthBinaryFixture{
		repoRoot:   f.repoRoot,
		binaryPath: f.binaryPath,
		env:        newSyntheticAuthProcessEnv(t),
		secrets:    make([][]byte, len(secrets)),
	}
	for index, secret := range secrets {
		fixture.secrets[index] = []byte(secret)
	}
	fixture.authPath = fixture.resolveAuthPath(t, fixture.env)
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
	env := newSyntheticAuthProcessEnv(t)
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

// newSyntheticAuthProcessEnv 在首次运行待测 binary 前关闭自动更新检查。
// packaged smoke 可指向较早的已发布 binary；其更新提示与 auth transfer
// 协议无关，不能污染本测试对 stdout/stderr 的精确断言。
func newSyntheticAuthProcessEnv(t *testing.T) isolatedProcessEnv {
	t.Helper()
	env := isolatedEnv(t)
	configRoot := env.configRoot
	if runtime.GOOS == "darwin" {
		// Darwin 的 os.UserConfigDir 固定使用 HOME 下的 Application Support。
		configRoot = filepath.Join(env.home, "Library", "Application Support")
	}
	configPath := filepath.Join(configRoot, "pixiv", "config.toml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o700); err != nil {
		t.Fatalf("create synthetic auth config directory: %v", err)
	}
	if err := os.WriteFile(configPath, []byte("[update]\ncheck_enabled = false\n"), 0o600); err != nil {
		t.Fatalf("write synthetic auth config: %v", err)
	}
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

func (f syntheticAuthBinaryFixture) requireOnlyStoredToken(t *testing.T, path string, userID int64, token string) {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read synthetic auth store: %v", err)
	}
	var store struct {
		Accounts []syntheticAuthAccount `json:"accounts"`
	}
	if err := json.Unmarshal(body, &store); err != nil {
		t.Fatalf("decode synthetic auth store: %v; body omitted", err)
	}
	if len(store.Accounts) != 1 {
		t.Fatalf("direct import auth store account count=%d, want 1", len(store.Accounts))
	}
	for _, account := range store.Accounts {
		if account.UserID == userID {
			if !bytes.Equal([]byte(account.RefreshToken), []byte(token)) {
				t.Fatalf("stored rotated token mismatch: uid=%d bytes=%d", userID, len(account.RefreshToken))
			}
			return
		}
	}
	t.Fatalf("stored imported account missing: uid=%d accounts=%d", userID, len(store.Accounts))
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
