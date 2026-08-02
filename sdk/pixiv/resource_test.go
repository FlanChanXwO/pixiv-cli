package pixiv

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/resource"
	"github.com/FlanChanXwO/pixiv-cli/internal/utils/atomicfile"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

func TestValidateResourceURL(t *testing.T) {
	client, _ := New("token")
	valid := []string{
		"https://i.pximg.net/c/250x250/img-master/img/2024/01/01/00/00/00/123_p0_master1200.jpg",
		"https://s.pximg.net/www/mypixiv/example.png",
		"https://i-f.pximg.net/c/540x540_10_webp/img-master/img/example.webp",
	}
	for _, raw := range valid {
		if err := client.validateResourceURL(raw); err != nil {
			t.Fatalf("validateResourceURL(%q): %v", raw, err)
		}
	}
	invalid := []string{
		"http://i.pximg.net/example.jpg",
		"https://evil.example.com/example.jpg",
		"https://i.pximg.net",
		"https://user:pass@i.pximg.net/example.jpg",
		"https://i.pximg.net/",
	}
	for _, raw := range invalid {
		if err := client.validateResourceURL(raw); err == nil {
			t.Fatalf("validateResourceURL(%q) should fail", raw)
		}
	}
}

func TestValidateResourceURLCustomHost(t *testing.T) {
	client, _ := NewWith("token", Options{ResourcePolicy: ResourcePolicy{AllowedHosts: []string{"cdn.internal.example"}}})
	if err := client.validateResourceURL("https://cdn.internal.example/img/1.png"); err != nil {
		t.Fatalf("custom host should be allowed: %v", err)
	}
}

func TestNewResourceBuildsRefAndHeaders(t *testing.T) {
	client, _ := New("token")
	res, err := client.newResource("artwork", 42, 0, "https://i.pximg.net/img/example.jpg")
	if err != nil {
		t.Fatalf("newResource: %v", err)
	}
	if res.URL != "https://i.pximg.net/img/example.jpg" {
		t.Fatalf("url = %q", res.URL)
	}
	if res.RequestHeaders["Referer"] == "" {
		t.Fatal("resource should carry a referer header")
	}
	if res.RequiresCredentials {
		t.Fatal("pixiv image resource should not require credentials")
	}
	payload, err := sdk.ResourceRefPayload(res.Ref)
	if err != nil {
		t.Fatalf("ResourceRefPayload: %v", err)
	}
	if !strings.Contains(string(payload), "42") {
		t.Fatalf("ref payload should encode identity: %q", payload)
	}
}

func TestOpenResourceStreamsWithHeaders(t *testing.T) {
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Referer") == "" {
			t.Error("referer not sent")
		}
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, "/final", http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Content-Length", "4")
		_, _ = w.Write([]byte("DATA"))
	}))
	defer upstream.Close()

	client := resourceTestClient(t, upstream)
	res, err := client.newResource("artwork", 1, 0, upstream.URL+"/final")
	if err != nil {
		t.Fatalf("newResource: %v", err)
	}
	response, err := client.OpenResource(context.Background(), sdk.OpenResourceRequest{Ref: res.Ref})
	if err != nil {
		t.Fatalf("OpenResource: %v", err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if string(body) != "DATA" {
		t.Fatalf("body = %q", body)
	}
	if response.ContentType() != "image/png" {
		t.Fatalf("content type = %q", response.ContentType())
	}
}

func TestSaveResourceAtomic(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "out", "img.png")
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("image-bytes"))
	}))
	defer upstream.Close()
	client := resourceTestClient(t, upstream)

	res, _ := client.newResource("artwork", 1, 0, upstream.URL+"/img.png")
	saved, err := client.SaveResource(context.Background(), res.Ref, sdk.SaveOptions{Path: dest})
	if err != nil {
		t.Fatalf("SaveResource: %v", err)
	}
	if saved.Size != int64(len("image-bytes")) {
		t.Fatalf("saved size = %d", saved.Size)
	}
	content, err := os.ReadFile(dest)
	if err != nil || string(content) != "image-bytes" {
		t.Fatalf("dest content = %q err=%v", content, err)
	}
	entries, _ := os.ReadDir(filepath.Dir(dest))
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".pixiv-resource-") {
			t.Fatalf("temp file left behind: %s", e.Name())
		}
	}
}

func resourceTestClient(t *testing.T, server *httptest.Server) *Client {
	t.Helper()
	parsed, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse server url: %v", err)
	}
	client, err := NewWith("token", Options{ResourcePolicy: ResourcePolicy{AllowedHosts: []string{parsed.Hostname()}}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}
	client.httpClient = server.Client()
	client.resClient = resource.NewApp(client.httpClient)
	return client
}

func TestAtomicWriteFailureRemovesTemp(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "out.txt")
	_, err := atomicfile.Write(context.Background(), dest, errorReader{err: errors.New("boom")})
	if err == nil {
		t.Fatal("expected error")
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatal("destination should not exist after failure")
	}
}

type errorReader struct{ err error }

func (r errorReader) Read([]byte) (int, error) { return 0, r.err }
