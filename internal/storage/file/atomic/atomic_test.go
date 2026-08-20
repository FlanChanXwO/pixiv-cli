package atomic_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	atomic "github.com/FlanChanXwO/pixiv-cli/internal/storage/file/atomic"
)

func TestWriteAtomic(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "out", "file.bin")
	written, err := atomic.Write(context.Background(), dest, strings.NewReader("hello"))
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if written != 5 {
		t.Fatalf("written = %d", written)
	}
	content, err := os.ReadFile(dest)
	if err != nil || string(content) != "hello" {
		t.Fatalf("content = %q err=%v", content, err)
	}
	entries, _ := os.ReadDir(filepath.Dir(dest))
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".atomic-write-") {
			t.Fatalf("temp file left behind: %s", e.Name())
		}
	}
}

func TestAtomicWriteIsCanonicalEntryPoint(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "nested", "atomic.txt")

	written, err := atomic.AtomicWrite(context.Background(), dest, strings.NewReader("hello"))
	if err != nil {
		t.Fatalf("AtomicWrite: %v", err)
	}
	if written != 5 {
		t.Fatalf("written = %d", written)
	}
	body, err := os.ReadFile(dest)
	if err != nil || string(body) != "hello" {
		t.Fatalf("content = %q err=%v", body, err)
	}
}

func TestWriteFailureRemovesTemp(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "out.txt")
	_, err := atomic.Write(context.Background(), dest, errorReader{err: errors.New("boom")})
	if err == nil {
		t.Fatal("expected error")
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatal("destination should not exist after failure")
	}
}

func TestWriteCanceledContext(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "out.txt")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := atomic.Write(ctx, dest, strings.NewReader("data")); err == nil {
		t.Fatal("expected context error")
	}
}

type errorReader struct{ err error }

func (r errorReader) Read([]byte) (int, error) { return 0, r.err }
