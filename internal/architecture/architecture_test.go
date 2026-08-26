package architecture_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const modulePath = "github.com/FlanChanXwO/pixiv-cli"

func TestReverseSearchBoundaryExceptionIsDocumented(t *testing.T) {
	repositoryRoot := findRepositoryRoot(t)
	wants := map[string][]string{
		"AGENTS.md": {
			"reverse-search is the only cross-boundary exception",
			"internal/services/reversesearch/assembly",
			"internal/services/reversesearch/saucenao",
			"internal/services/reversesearch/ascii2d",
			"internal/cli/commands",
			"internal/mcpserver",
		},
		"docs/en/maintainers/architecture.md": {
			"### Reverse-search Facade exception",
			"internal/services/reversesearch/assembly",
			"internal/services/reversesearch/saucenao",
			"internal/services/reversesearch/ascii2d",
			"internal/cli/commands",
			"internal/mcpserver",
		},
		"docs/zh-CN/maintainers/architecture.md": {
			"### reverse-search Facade 例外",
			"internal/services/reversesearch/assembly",
			"internal/services/reversesearch/saucenao",
			"internal/services/reversesearch/ascii2d",
			"internal/cli/commands",
			"internal/mcpserver",
		},
	}
	for relativePath, phrases := range wants {
		body, err := os.ReadFile(filepath.Join(repositoryRoot, relativePath))
		if err != nil {
			t.Fatalf("read %s: %v", relativePath, err)
		}
		text := strings.ToLower(string(body))
		for _, phrase := range phrases {
			if !strings.Contains(text, strings.ToLower(phrase)) {
				t.Errorf("%s is missing architecture rule phrase %q", relativePath, phrase)
			}
		}
	}
}

func TestCLIAndMCPDoNotImportReverseSearchSubpackages(t *testing.T) {
	repositoryRoot := findRepositoryRoot(t)
	for _, relativeDir := range []string{"internal/cli/commands", "internal/mcpserver", "internal/shared/record"} {
		directory := filepath.Join(repositoryRoot, relativeDir)
		err := filepath.WalkDir(directory, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
				return nil
			}
			file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
			if err != nil {
				return err
			}
			for _, importSpec := range file.Imports {
				importPath, err := strconv.Unquote(importSpec.Path.Value)
				if err != nil {
					return err
				}
				if strings.HasPrefix(importPath, modulePath+"/internal/services/reversesearch/") {
					t.Errorf("%s imports reverse-search subpackage %q; only the top-level contract is allowed here", path, importPath)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("scan %s: %v", relativeDir, err)
		}
	}
}

func findRepositoryRoot(t *testing.T) string {
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
