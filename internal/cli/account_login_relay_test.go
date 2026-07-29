package cli

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/cli/loginhelper"
	"github.com/FlanChanXwO/pixiv-cli/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRelayLoginServerAuthenticatesAndAcceptsOneMatchingCallback(t *testing.T) {
	addr := freeLoopbackAddr(t)
	const secret = "relay-secret-must-not-appear"
	const callback = "pixiv://account/login?code=remote-one-time-code"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a := app{errOut: io.Discard}
	type outcome struct {
		code    string
		cleanup func()
		err     error
	}
	resultCh := make(chan outcome, 1)
	go func() {
		code, _, cleanup, err := a.waitForRelayLoginCode(ctx, loginRelayServerOptions{
			PublicURL:  "http://" + addr,
			ListenAddr: addr,
			Secret:     secret,
		}, func(raw string) bool { return raw == callback }, "https://app-api.pixiv.net/web/v1/login?state=test")
		resultCh <- outcome{code: code, cleanup: cleanup, err: err}
	}()
	t.Cleanup(func() {
		select {
		case result := <-resultCh:
			if result.cleanup != nil {
				result.cleanup()
			}
		default:
			cancel()
		}
	})

	waitForLoginServer(t, addr)
	unauthorized, err := http.Get("http://" + addr + "/session")
	require.NoError(t, err)
	require.Equal(t, http.StatusUnauthorized, unauthorized.StatusCode)
	_ = unauthorized.Body.Close()

	wrong, err := newRelayCallbackRequest("http://"+addr+"/callback", "wrong-secret", callback)
	require.NoError(t, err)
	wrongResponse, err := http.DefaultClient.Do(wrong)
	require.NoError(t, err)
	require.Equal(t, http.StatusUnauthorized, wrongResponse.StatusCode)
	_ = wrongResponse.Body.Close()

	valid, err := newRelayCallbackRequest("http://"+addr+"/callback", secret, callback)
	require.NoError(t, err)
	validResponse, err := http.DefaultClient.Do(valid)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, validResponse.StatusCode)
	require.NotEmpty(t, validResponse.Header.Get(loginhelper.RelayResultURLHeader))
	_ = validResponse.Body.Close()

	repeat, err := newRelayCallbackRequest("http://"+addr+"/callback", secret, callback)
	require.NoError(t, err)
	repeatResponse, err := http.DefaultClient.Do(repeat)
	require.NoError(t, err)
	require.Equal(t, http.StatusConflict, repeatResponse.StatusCode)
	_ = repeatResponse.Body.Close()

	select {
	case result := <-resultCh:
		require.NoError(t, result.err)
		assert.Equal(t, callback, result.code)
		result.cleanup()
	case <-time.After(5 * time.Second):
		t.Fatal("remote relay did not deliver callback")
	}
}

// 远程 callback 不应只返回 API 状态；浏览器机器必须能获得一次性结果页，
// 并在服务器完成 OAuth exchange 后看到与本地登录相同的最终成功反馈。
func TestRelayLoginServerPublishesFinalBrowserPage(t *testing.T) {
	addr := freeLoopbackAddr(t)
	const secret = "relay-final-page-secret"
	const callback = "pixiv://account/login?code=remote-final-page-code"
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	a := app{errOut: io.Discard}
	type outcome struct {
		code    string
		notify  func(bool)
		cleanup func()
		err     error
	}
	resultCh := make(chan outcome, 1)
	go func() {
		code, notify, cleanup, err := a.waitForRelayLoginCode(ctx, loginRelayServerOptions{
			PublicURL:  "http://" + addr,
			ListenAddr: addr,
			Secret:     secret,
		}, func(raw string) bool { return raw == callback }, "https://app-api.pixiv.net/web/v1/login?state=test")
		resultCh <- outcome{code: code, notify: notify, cleanup: cleanup, err: err}
	}()
	t.Cleanup(func() {
		select {
		case result := <-resultCh:
			if result.cleanup != nil {
				result.cleanup()
			}
		default:
		}
	})

	waitForLoginServer(t, addr)
	request, err := newRelayCallbackRequest("http://"+addr+"/callback", secret, callback)
	require.NoError(t, err)
	callbackResponse, err := http.DefaultClient.Do(request)
	require.NoError(t, err)
	defer callbackResponse.Body.Close()
	require.Equal(t, http.StatusOK, callbackResponse.StatusCode)
	resultURL := callbackResponse.Header.Get("X-Pixiv-Relay-Result-URL")
	require.NotEmpty(t, resultURL)

	type pageResult struct {
		body string
		err  error
	}
	pageCh := make(chan pageResult, 1)
	go func() {
		response, getErr := http.Get(resultURL)
		if getErr != nil {
			pageCh <- pageResult{err: getErr}
			return
		}
		defer response.Body.Close()
		body, readErr := io.ReadAll(response.Body)
		if response.StatusCode != http.StatusOK && readErr == nil {
			readErr = fmt.Errorf("result page status = %d", response.StatusCode)
		}
		pageCh <- pageResult{body: string(body), err: readErr}
	}()

	select {
	case result := <-resultCh:
		require.NoError(t, result.err)
		assert.Equal(t, callback, result.code)
		result.notify(true)
		result.cleanup()
	case <-time.After(5 * time.Second):
		t.Fatal("remote relay did not deliver callback")
	}

	select {
	case page := <-pageCh:
		require.NoError(t, page.err)
		assert.Contains(t, page.body, "Login successful")
	case <-time.After(5 * time.Second):
		t.Fatal("remote relay did not publish final browser page")
	}
}

func TestRelayLoginServerPublishesFinalFailureBrowserPage(t *testing.T) {
	addr := freeLoopbackAddr(t)
	const secret = "relay-final-failure-page-secret"
	const callback = "pixiv://account/login?code=remote-final-failure-code"
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	a := app{errOut: io.Discard}
	type outcome struct {
		code    string
		notify  func(bool)
		cleanup func()
		err     error
	}
	resultCh := make(chan outcome, 1)
	go func() {
		code, notify, cleanup, err := a.waitForRelayLoginCode(ctx, loginRelayServerOptions{
			PublicURL:  "http://" + addr,
			ListenAddr: addr,
			Secret:     secret,
		}, func(raw string) bool { return raw == callback }, "https://app-api.pixiv.net/web/v1/login?state=test")
		resultCh <- outcome{code: code, notify: notify, cleanup: cleanup, err: err}
	}()
	t.Cleanup(func() {
		select {
		case result := <-resultCh:
			if result.cleanup != nil {
				result.cleanup()
			}
		default:
		}
	})

	waitForLoginServer(t, addr)
	request, err := newRelayCallbackRequest("http://"+addr+"/callback", secret, callback)
	require.NoError(t, err)
	callbackResponse, err := http.DefaultClient.Do(request)
	require.NoError(t, err)
	defer callbackResponse.Body.Close()
	require.Equal(t, http.StatusOK, callbackResponse.StatusCode)
	resultURL := callbackResponse.Header.Get(loginhelper.RelayResultURLHeader)
	require.NotEmpty(t, resultURL)

	type pageResult struct {
		status int
		body   string
		err    error
	}
	pageCh := make(chan pageResult, 1)
	go func() {
		response, getErr := http.Get(resultURL)
		if getErr != nil {
			pageCh <- pageResult{err: getErr}
			return
		}
		defer response.Body.Close()
		body, readErr := io.ReadAll(response.Body)
		pageCh <- pageResult{status: response.StatusCode, body: string(body), err: readErr}
	}()

	select {
	case result := <-resultCh:
		require.NoError(t, result.err)
		assert.Equal(t, callback, result.code)
		result.notify(false)
		result.cleanup()
	case <-time.After(5 * time.Second):
		t.Fatal("remote relay did not deliver callback")
	}

	select {
	case page := <-pageCh:
		require.NoError(t, page.err)
		assert.Equal(t, http.StatusBadRequest, page.status)
		assert.Contains(t, page.body, "Login failed")
		assert.NotContains(t, strings.ToLower(page.body), "secret")
	case <-time.After(5 * time.Second):
		t.Fatal("remote relay did not publish final failure browser page")
	}
}

func newRelayCallbackRequest(endpoint, secret, callback string) (*http.Request, error) {
	body, err := json.Marshal(relayLoginCallbackRequest{CallbackURL: callback})
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+secret)
	request.Header.Set("Content-Type", "application/json")
	return request, nil
}

func TestConfiguredRelayServerOptionsRequireCompleteConfiguration(t *testing.T) {
	flags := changedFlags{}
	_, enabled, err := configuredRelayServerOptions(flags, accountLoginOptions{}, config.RuntimeConfig{LoginRelayPublicURL: "https://relay.example", LoginRelaySecret: "canary-secret-value"})
	require.Error(t, err)
	assert.False(t, enabled)
	assert.NotContains(t, err.Error(), "canary-secret-value")
}

func TestRelayLoginServerSupportsDirectTLS(t *testing.T) {
	addr := freeLoopbackAddr(t)
	certFile, keyFile := writeRelayTLSCertificate(t)
	const secret = "tls-relay-secret-canary"
	const callback = "pixiv://account/login?code=tls-one-time-code"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	a := app{errOut: io.Discard}
	type outcome struct {
		code    string
		cleanup func()
		err     error
	}
	resultCh := make(chan outcome, 1)
	go func() {
		code, _, cleanup, err := a.waitForRelayLoginCode(ctx, loginRelayServerOptions{
			PublicURL:   "https://" + addr,
			ListenAddr:  addr,
			Secret:      secret,
			TLSCertFile: certFile,
			TLSKeyFile:  keyFile,
		}, func(raw string) bool { return raw == callback }, "https://app-api.pixiv.net/web/v1/login?state=test")
		resultCh <- outcome{code: code, cleanup: cleanup, err: err}
	}()
	t.Cleanup(func() {
		select {
		case result := <-resultCh:
			if result.cleanup != nil {
				result.cleanup()
			}
		default:
			cancel()
		}
	})

	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}} // #nosec G402 -- locally generated test certificate
	waitForRelayTLSServer(t, client, addr)

	session, err := http.NewRequest(http.MethodGet, "https://"+addr+"/session", nil)
	require.NoError(t, err)
	session.Header.Set("Authorization", "Bearer "+secret)
	sessionResponse, err := client.Do(session)
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, sessionResponse.StatusCode)
	_ = sessionResponse.Body.Close()

	valid, err := newRelayCallbackRequest("https://"+addr+"/callback", secret, callback)
	require.NoError(t, err)
	validResponse, err := client.Do(valid)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, validResponse.StatusCode)
	require.NotEmpty(t, validResponse.Header.Get(loginhelper.RelayResultURLHeader))
	_ = validResponse.Body.Close()

	select {
	case result := <-resultCh:
		require.NoError(t, result.err)
		assert.Equal(t, callback, result.code)
		result.cleanup()
	case <-time.After(5 * time.Second):
		t.Fatal("TLS remote relay did not deliver callback")
	}
}

func waitForRelayTLSServer(t *testing.T, client *http.Client, addr string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		response, err := client.Get("https://" + addr + "/session")
		if err == nil {
			_ = response.Body.Close()
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("TLS relay server did not start at %s", addr)
}

func writeRelayTLSCertificate(t *testing.T) (string, string) {
	t.Helper()
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	require.NoError(t, err)
	keyDER, err := x509.MarshalECPrivateKey(privateKey)
	require.NoError(t, err)
	dir := t.TempDir()
	certFile := filepath.Join(dir, "relay-cert.pem")
	keyFile := filepath.Join(dir, "relay-key.pem")
	require.NoError(t, os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600))
	require.NoError(t, os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600))
	return certFile, keyFile
}

type changedFlags map[string]bool

func (f changedFlags) Changed(name string) bool { return f[name] }
