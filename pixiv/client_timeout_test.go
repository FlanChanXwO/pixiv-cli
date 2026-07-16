package pixiv

import (
	"net/http"
	"testing"
	"time"
)

func TestNewClientUsesDedicatedClientWithoutTotalTimeout(t *testing.T) {
	client, err := NewClient(Options{})
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}
	if client.httpClient == nil {
		t.Fatal("HTTP client is nil")
	}
	if client.httpClient == http.DefaultClient {
		t.Fatal("HTTP client unexpectedly aliases http.DefaultClient")
	}
	if client.httpClient.Timeout != 0 {
		t.Fatalf("timeout = %v, want zero", client.httpClient.Timeout)
	}
}

func TestNewClientPreservesExplicitHTTPClient(t *testing.T) {
	want := &http.Client{Timeout: 37 * time.Second}
	client, err := NewClient(Options{HTTPClient: want})
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}
	if client.httpClient != want || client.httpClient.Timeout != want.Timeout {
		t.Fatalf("HTTP client = %p timeout %v, want %p timeout %v", client.httpClient, client.httpClient.Timeout, want, want.Timeout)
	}
}
