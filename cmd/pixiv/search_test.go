package main_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestSearchOutputContract 固定 search 的文本、JSON 与 NDJSON 契约。请求经本地
// CONNECT 代理与 TLS 夹具，账号与 token 轮换使用真实 SQLite 与 SDK。
func TestSearchOutputContract(t *testing.T) {
	server := newFixtureServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/token":
			fmt.Fprint(w, `{"access_token":"access","refresh_token":"rotated","expires_in":3600,"user":{"id":"12345678","name":"artist"}}`)
		case "/v1/search/illust":
			fmt.Fprint(w, `{"illusts":[{"id":101,"title":"cat","type":"illust","create_date":"2024-05-01T10:00:00+09:00","user":{"id":7,"name":"artist"},"tags":[]}],"next_url":null}`)
		default:
			http.NotFound(w, r)
		}
	}))
	home := isolatedHome(t)
	seedPixivAccount(t, home, "artist", false)

	for _, format := range []string{"text", "json", "ndjson"} {
		t.Run(format, func(t *testing.T) {
			args := []string{"search", "cat", "--limit", "1", "--proxy", server.proxyURL}
			if format != "text" {
				args = append(args, "--"+format)
			}
			stdout, stderr, code := runCLI(t, args...)
			require.Equal(t, 0, code, stderr)
			require.Empty(t, stderr)
			want, err := os.ReadFile(filepath.Join("testdata", "search-"+format+".golden"))
			require.NoError(t, err)
			require.Equal(t, string(want), stdout)
		})
	}
}

type fixtureServer struct {
	proxyURL string
}

// newFixtureServer 用覆盖 app-api 与 OAuth 主机的证书启动 TLS 服务，再以 CONNECT
// 代理把所有连接导向它；CLI 仍然按真实主机名完成 TLS 校验。
func newFixtureServer(t *testing.T, handler http.Handler) fixtureServer {
	t.Helper()
	certificate, pool := fixtureCertificate(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	server := httptest.NewUnstartedServer(handler)
	server.Listener = listener
	server.TLS = &tls.Config{Certificates: []tls.Certificate{certificate}}
	server.StartTLS()
	t.Cleanup(server.Close)

	caPath := filepath.Join(t.TempDir(), "fixture-ca.pem")
	require.NoError(t, os.WriteFile(caPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: pool.Raw}), 0o600))
	t.Setenv("SSL_CERT_FILE", caPath)

	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect {
			http.Error(w, "CONNECT required", http.StatusMethodNotAllowed)
			return
		}
		target, err := net.Dial("tcp", server.Listener.Addr().String())
		if err != nil {
			http.Error(w, "fixture unavailable", http.StatusBadGateway)
			return
		}
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "hijacking unsupported", http.StatusInternalServerError)
			return
		}
		client, _, err := hijacker.Hijack()
		if err != nil {
			http.Error(w, "hijack failed", http.StatusInternalServerError)
			return
		}
		defer client.Close()
		defer target.Close()
		_, _ = io.WriteString(client, "HTTP/1.1 200 Connection Established\r\n\r\n")
		go func() { _, _ = io.Copy(target, client) }()
		_, _ = io.Copy(client, target)
	}))
	t.Cleanup(proxy.Close)
	return fixtureServer{proxyURL: proxy.URL}
}

// fixtureCertificate 生成 app-api 与 OAuth 主机的自签名证书。
func fixtureCertificate(t *testing.T) (tls.Certificate, *x509.Certificate) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "app-api.pixiv.net"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		DNSNames:              []string{"app-api.pixiv.net", "oauth.secure.pixiv.net"},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)
	certificate, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: certificate}, certificate
}
