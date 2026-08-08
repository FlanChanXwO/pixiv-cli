package architecture_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

const modulePath = "github.com/FlanChanXwO/pixiv-cli"

func TestInternalArchitectureHasNoRetiredDirectories(t *testing.T) {
	root := repositoryRoot(t)
	for _, relative := range []string{
		"internal/config",
		"internal/download",
		"internal/platform",
		"internal/storage",
		"internal/testutil/socks5test",
		"internal/services/pixiv/doc.go",
	} {
		if _, err := os.Stat(filepath.Join(root, relative)); err == nil {
			t.Errorf("retired path still exists: %s", relative)
		} else if !os.IsNotExist(err) {
			t.Errorf("stat retired path %s: %v", relative, err)
		}
	}
	for _, relative := range []string{"internal/mcpserver/legacy_tools.go", "internal/update/command.go"} {
		if _, err := os.Stat(filepath.Join(root, relative)); err == nil {
			t.Errorf("retired file still exists: %s", relative)
		} else if !os.IsNotExist(err) {
			t.Errorf("stat retired file %s: %v", relative, err)
		}
	}
}

func TestInternalArchitectureHasExpectedCommandAndProductPackages(t *testing.T) {
	root := repositoryRoot(t)
	for _, relative := range []string{
		"internal/cli/auth/command.go",
		"internal/cli/config/command.go",
		"internal/cli/pixiv/command.go",
		"internal/cli/download/command.go",
		"internal/cli/fanbox/command.go",
		"internal/cli/mcp/command.go",
		"internal/cli/update/command.go",
		"internal/cli/version/command.go",
		"internal/mcpserver/pixiv/server.go",
		"internal/mcpserver/fanbox/server.go",
	} {
		if info, err := os.Stat(filepath.Join(root, relative)); err != nil {
			t.Errorf("required architecture package file %s: %v", relative, err)
		} else if !info.Mode().IsRegular() {
			t.Errorf("required architecture package file %s is not regular", relative)
		}
	}
	for _, relative := range []string{"internal/mcpserver/legacy_tools.go", "internal/mcpserver/server.go"} {
		if relative == "internal/mcpserver/server.go" {
			continue
		}
		if _, err := os.Stat(filepath.Join(root, relative)); err == nil {
			t.Errorf("product-split MCP package still has ambiguous file: %s", relative)
		}
	}
}

func TestInternalArchitectureHasNoForbiddenProductionImports(t *testing.T) {
	root := repositoryRoot(t)
	violations := make([]string, 0)
	err := filepath.WalkDir(filepath.Join(root, "internal"), func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		imports, err := productionImports(path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		pkgPath := modulePath + "/" + filepath.ToSlash(filepath.Dir(relative))
		for _, imported := range imports {
			switch {
			case strings.HasPrefix(pkgPath, modulePath+"/internal/cli") || strings.HasPrefix(pkgPath, modulePath+"/internal/mcpserver"):
				if forbiddenForAdapters(pkgPath, imported) {
					violations = append(violations, relative+" imports "+imported)
				}
			case strings.HasPrefix(pkgPath, modulePath+"/internal/application"):
				if forbiddenForApplication(imported) {
					violations = append(violations, relative+" imports "+imported)
				}
			case strings.HasPrefix(pkgPath, modulePath+"/internal/utils"):
				if forbiddenForUtils(imported) {
					violations = append(violations, relative+" imports "+imported)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(violations)
	for _, violation := range violations {
		t.Error(violation)
	}
}

func TestCommandGoBelongsToCLICommandPackages(t *testing.T) {
	root := repositoryRoot(t)
	var invalid []string
	err := filepath.WalkDir(filepath.Join(root, "internal"), func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || entry.Name() != "command.go" {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		slashPath := filepath.ToSlash(relative)
		if !strings.HasPrefix(slashPath, "internal/cli/") || strings.Count(slashPath, "/") != 3 {
			invalid = append(invalid, slashPath)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range invalid {
		t.Errorf("command.go must be a direct CLI command package file: %s", path)
	}
}

func TestApplicationRootContainsOnlySharedPorts(t *testing.T) {
	root := repositoryRoot(t)
	entries, err := os.ReadDir(filepath.Join(root, "internal/application"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		if entry.Name() != "ports.go" {
			t.Errorf("application root implementation file must move to a use-case package: %s", entry.Name())
		}
	}
}

func TestCLIRootContainsOnlyRootControllerProductionFile(t *testing.T) {
	root := repositoryRoot(t)
	entries, err := os.ReadDir(filepath.Join(root, "internal/cli"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		if entry.Name() != "root.go" {
			t.Errorf("CLI root production logic must stay in root.go: %s", entry.Name())
		}
	}
}

func TestCLIChildrenDoNotImportRootPackage(t *testing.T) {
	root := repositoryRoot(t)
	err := filepath.WalkDir(filepath.Join(root, "internal/cli"), func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if filepath.ToSlash(filepath.Dir(relative)) == "internal/cli" {
			return nil
		}
		imports, err := productionImports(path)
		if err != nil {
			return err
		}
		for _, imported := range imports {
			if imported == modulePath+"/internal/cli" {
				t.Errorf("CLI child %s must not import root package", filepath.ToSlash(relative))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve architecture test location")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "../.."))
}

func productionImports(path string) ([]string, error) {
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
	if err != nil {
		return nil, err
	}
	imports := make([]string, 0, len(file.Imports))
	for _, spec := range file.Imports {
		imports = append(imports, strings.Trim(spec.Path.Value, `"`))
	}
	return imports, nil
}

func forbiddenForAdapters(pkgPath, imported string) bool {
	return strings.HasPrefix(imported, modulePath+"/internal/services") ||
		strings.HasPrefix(imported, modulePath+"/internal/persistence") ||
		strings.HasPrefix(imported, modulePath+"/internal/storage") ||
		strings.HasPrefix(imported, modulePath+"/internal/platform") ||
		(strings.HasPrefix(imported, modulePath+"/internal/filesystem") && !strings.HasPrefix(pkgPath, modulePath+"/internal/cli/auth"))
}

func forbiddenForApplication(imported string) bool {
	return strings.HasPrefix(imported, modulePath+"/internal/services") ||
		strings.HasPrefix(imported, modulePath+"/internal/persistence") ||
		strings.HasPrefix(imported, modulePath+"/internal/storage") ||
		strings.HasPrefix(imported, modulePath+"/internal/platform") ||
		strings.HasPrefix(imported, modulePath+"/internal/network") ||
		strings.HasPrefix(imported, modulePath+"/internal/browsercookies") ||
		strings.HasPrefix(imported, modulePath+"/internal/downloader") ||
		strings.HasPrefix(imported, modulePath+"/internal/cli")
}

func forbiddenForUtils(imported string) bool {
	return strings.HasPrefix(imported, modulePath+"/internal/services") ||
		strings.HasPrefix(imported, modulePath+"/internal/persistence") ||
		strings.HasPrefix(imported, modulePath+"/internal/storage") ||
		strings.HasPrefix(imported, modulePath+"/internal/filesystem") ||
		strings.HasPrefix(imported, modulePath+"/internal/platform") ||
		strings.HasPrefix(imported, modulePath+"/internal/application") ||
		strings.HasPrefix(imported, modulePath+"/internal/cli")
}
