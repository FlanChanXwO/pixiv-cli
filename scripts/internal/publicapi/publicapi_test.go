package publicapi

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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

func TestInventoryIgnoresMissingDirectory(t *testing.T) {
	t.Parallel()

	inventory := Inventory(filepath.Join(t.TempDir(), "missing"))
	for _, pkg := range []string{"sdk", "sdk/pixiv", "sdk/fanbox"} {
		if len(inventory[pkg]) != 0 {
			t.Fatalf("Inventory(missing dir)[%q] = %v, want empty", pkg, inventory[pkg])
		}
	}
}
