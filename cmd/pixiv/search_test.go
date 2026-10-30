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
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestSearchOutputContract 固定 search 的文本、JSON 与 NDJSON 契约。请求经本地
// CONNECT 代理与 TLS 夹具，账号与 token 轮换使用真实 SQLite 与 SDK。
func TestSearchOutputContract(t *testing.T) {
	proxy := fixtureProxy(t)
	home := isolatedHome(t)
	seedPixivAccount(t, home, "artist", false)
	assertSearchOutput(t, proxy)
}

func fixtureProxy(t *testing.T) string {
	t.Helper()
	certificate, err := fixtureCertificate()
	require.NoError(t, err)
	server := httptest.NewUnstartedServer(http.HandlerFunc(pixivHandler))
	server.TLS = &tls.Config{Certificates: []tls.Certificate{certificate}}
	server.StartTLS()
	t.Cleanup(server.Close)
	caPath := filepath.Join(t.TempDir(), "fixture-ca.pem")
	require.NoError(t, os.WriteFile(caPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate.Certificate[0]}), 0o600))
	t.Setenv("SSL_CERT_FILE", caPath)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxyPixiv(w, r, server.Listener.Addr().String())
	}))
	t.Cleanup(proxy.Close)
	return proxy.URL
}

func assertSearchOutput(t *testing.T, proxyURL string) {
	t.Helper()
	for _, format := range []string{"text", "json", "ndjson"} {
		t.Run(format, func(t *testing.T) {
			args := []string{"search", "cat", "--limit", "1", "--proxy", proxyURL}
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

// proxyPixiv 将 CONNECT 隧道转发到本地 TLS 服务，保留真实主机名校验。
func proxyPixiv(w http.ResponseWriter, _ *http.Request, address string) {
	target, err := net.Dial("tcp", address)
	if err != nil {
		http.Error(w, "fixture unavailable", http.StatusBadGateway)
		return
	}
	defer target.Close()
	client, _, err := w.(http.Hijacker).Hijack()
	if err != nil {
		http.Error(w, "hijack failed", http.StatusInternalServerError)
		return
	}
	defer client.Close()
	_, _ = io.WriteString(client, "HTTP/1.1 200 Connection Established\r\n\r\n")
	go func() { _, _ = io.Copy(target, client) }()
	_, _ = io.Copy(client, target)
}

func pixivHandler(w http.ResponseWriter, r *http.Request) {
	artwork := `{"id":101,"title":"line\n\u001b[31m","type":"illust","create_date":"2024-05-01T10:00:00+09:00","user":{"id":7,"name":"line\n\u001b[31m"},"tags":[{"name":"line\n\u001b[31m"}],"caption":"line<br>next"}`
	novel := `{"x_restrict":0,"text_length":1,"is_original":false,"id":101,"title":"line\n\u001b[31m","create_date":"2024-05-01T10:00:00+09:00","user":{"id":7,"name":"artist"},"tags":[]}`
	switch r.URL.Path {
	case "/auth/token":
		fmt.Fprint(w, `{"access_token":"access","refresh_token":"rotated","expires_in":3600,"user":{"id":"12345678","name":"artist"}}`)
	case "/creator.listFollowing":
		fmt.Fprint(w, `{"body":{"creators":[{"creatorId":"line\n\u001b[31m"}]}}`)
	case "/post.listCreator":
		fmt.Fprint(w, `{"body":{"posts":[{"id":"101","title":"line\n\u001b[31m","creatorId":"artist","publishedDatetime":"2024-05-01T00:00:00Z"}]}}`)
	case "/v1/illust/detail":
		fmt.Fprintf(w, `{"illust":%s}`, artwork)
	case "/v2/novel/detail":
		fmt.Fprintf(w, `{"novel":%s}`, novel)
	case "/v1/search/novel":
		fmt.Fprintf(w, `{"novels":[%s],"next_url":null}`, novel)
	case "/v3/illust/comments":
		fmt.Fprint(w, `{"comments":[{"id":1,"comment":"line\n\u001b[31m","date":"2024-05-01T10:00:00+09:00","user":{"id":7,"name":"artist"}}],"next_url":null}`)
	case "/v1/user/bookmark-tags/illust":
		fmt.Fprint(w, `{"bookmark_tags":[{"name":"line\n\u001b[31m","count":1}],"next_url":null}`)
	case "/v1/search/illust":
		if r.URL.Query().Get("word") == "unsafe" {
			fmt.Fprintf(w, `{"illusts":[%s],"next_url":null}`, artwork)
			return
		}
		fmt.Fprint(w, `{"illusts":[{"id":101,"title":"cat","type":"illust","create_date":"2024-05-01T10:00:00+09:00","user":{"id":7,"name":"artist"},"tags":[]}],"next_url":null}`)
	default:
		http.NotFound(w, r)
	}
}

var fixtureCertificate = sync.OnceValues(func() (tls.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "app-api.pixiv.net"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true, IsCA: true,
		DNSNames: []string{"app-api.pixiv.net", "oauth.secure.pixiv.net", "api.fanbox.cc"},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, err
	}

	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, nil
})
