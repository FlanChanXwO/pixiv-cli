package pixiv_test

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/pixiv"
)

func TestNewClientUsesHTTP1ForResourcesWithExplicitProxy(t *testing.T) {
	protocols := make(chan string, 1)
	resourceServer := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		protocols <- r.Proto
		_, _ = io.WriteString(w, "resource")
	}))
	resourceServer.EnableHTTP2 = true
	resourceServer.TLS = &tls.Config{NextProtos: []string{"h2", "http/1.1"}}
	resourceServer.StartTLS()
	defer resourceServer.Close()

	proxyServer := httptest.NewServer(http.HandlerFunc(tunnelProxy))
	defer proxyServer.Close()
	proxyURL, err := url.Parse(proxyServer.URL)
	if err != nil {
		t.Fatalf("parse proxy URL: %v", err)
	}
	resourceURL, err := url.Parse(resourceServer.URL)
	if err != nil {
		t.Fatalf("parse resource URL: %v", err)
	}
	trustedTransport := resourceServer.Client().Transport.(*http.Transport)
	transport := &http.Transport{
		TLSClientConfig:   trustedTransport.TLSClientConfig.Clone(),
		Proxy:             http.ProxyURL(proxyURL),
		ForceAttemptHTTP2: true,
	}

	client, err := pixiv.NewClient(pixiv.Options{
		HTTPClient: transportClient(transport),
		ResourcePolicy: pixiv.ResourcePolicy{Mirrors: []pixiv.ResourceMirrorPolicy{{
			Host:         resourceURL.Host,
			PathPrefixes: []string{"/resource"},
		}}},
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	ref, err := client.ParseResourceRef(resourceServer.URL + "/resource/image.jpg")
	if err != nil {
		t.Fatalf("parse resource ref: %v", err)
	}
	response, err := client.OpenResource(context.Background(), pixiv.OpenResourceRequest{Ref: ref})
	if err != nil {
		t.Fatalf("open resource: %v", err)
	}
	if err := response.Body.Close(); err != nil {
		t.Fatalf("close resource response: %v", err)
	}

	protocol := <-protocols
	if protocol != "HTTP/1.1" {
		t.Fatalf("resource protocol = %s, want HTTP/1.1 when an explicit proxy is configured", protocol)
	}
}

func transportClient(transport *http.Transport) *http.Client {
	return &http.Client{Transport: transport}
}

func tunnelProxy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodConnect {
		http.Error(w, "CONNECT required", http.StatusMethodNotAllowed)
		return
	}
	upstream, err := net.Dial("tcp", r.Host)
	if err != nil {
		http.Error(w, "cannot connect upstream", http.StatusBadGateway)
		return
	}
	defer upstream.Close()

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "hijacking unsupported", http.StatusInternalServerError)
		return
	}
	client, buffered, err := hijacker.Hijack()
	if err != nil {
		return
	}
	defer client.Close()
	_, _ = fmt.Fprint(client, "HTTP/1.1 200 Connection Established\r\n\r\n")
	if pending := buffered.Reader.Buffered(); pending != 0 {
		_, _ = io.CopyN(upstream, buffered.Reader, int64(pending))
	}

	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(upstream, client)
		close(done)
	}()
	_, _ = io.Copy(client, upstream)
	<-done
}
