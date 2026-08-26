package publicapi

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRepositoryPublicAPIInventoryIsPinned(t *testing.T) {
	repositoryRoot := findPublicAPIRepositoryRoot(t)
	digest := sha256.Sum256([]byte(Render(Inventory(repositoryRoot))))
	const wantDigest = "ed45ee60aba67e2a657174325e9796451a6ef88f4161dc643ad97368f5e7eb31"
	if got := hex.EncodeToString(digest[:]); got != wantDigest {
		t.Fatalf("public SDK inventory changed: got sha256 %s, want %s; update the inventory deliberately with the public API review", got, wantDigest)
	}
}

func findPublicAPIRepositoryRoot(t *testing.T) string {
	t.Helper()
	directory, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	for {
		if info, err := os.Stat(filepath.Join(directory, "go.mod")); err == nil && info.Mode().IsRegular() {
			return directory
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			t.Fatal("could not find repository root")
		}
		directory = parent
	}
}

func TestInventoryCollectsOnlyExportedPackageSymbols(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	sdkDir := filepath.Join(directory, "sdk")
	if err := os.MkdirAll(sdkDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFixture := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(sdkDir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeFixture("sdk.go", `package sdk
import "context"
type Service struct{}
func (s *Service) hidden() {}
func New() *Service { return nil }
func Run(ctx context.Context) error { return nil }
`)
	writeFixture("internal.go", `package sdk
func unexported() {}
`)
	writeFixture("sdk_test.go", `package sdk_test
func ExportedFromTest() {}
`)

	inventory := Inventory(directory)
	got := Render(inventory)
	wantLines := []string{
		"## sdk",
		"- New",
		"- Run",
		"- Service",
		"## sdk/pixiv",
		"## sdk/fanbox",
	}
	for _, want := range wantLines {
		if !strings.Contains(got, want+"\n") {
			t.Fatalf("render inventory = %q, missing line %q", got, want)
		}
	}
	if strings.Contains(got, "hidden") || strings.Contains(got, "unexported") || strings.Contains(got, "ExportedFromTest") {
		t.Fatalf("render inventory leaked non-exported or test symbols: %q", got)
	}
}
