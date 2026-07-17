package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeRejectsUnsafeKnowledgeArticleSourcesBeforeWritingArtifacts(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*testing.T, string) (string, string)
		want  string
	}{
		{
			name: "lexical parent traversal",
			setup: func(t *testing.T, root string) (string, string) {
				secret := "LEXICAL_OUTSIDE_CONTENT_MUST_NOT_BE_REPORTED"
				require.NoError(t, os.WriteFile(filepath.Join(root, "outside.md"), []byte(secret), 0o600))
				return "../outside.md", secret
			},
			want: "outside docs",
		},
		{
			name: "absolute path",
			setup: func(t *testing.T, root string) (string, string) {
				secret := "ABSOLUTE_OUTSIDE_CONTENT_MUST_NOT_BE_REPORTED"
				outsidePath := filepath.Join(root, "absolute-outside.md")
				require.NoError(t, os.WriteFile(outsidePath, []byte(secret), 0o600))
				return outsidePath, secret
			},
			want: "outside docs",
		},
		{
			name: "invalid UTF-8",
			setup: func(t *testing.T, root string) (string, string) {
				secret := "INVALID_UTF8_CONTENT_MUST_NOT_BE_REPORTED"
				content := append([]byte(secret), 0xff)
				require.NoError(t, os.WriteFile(filepath.Join(root, "docs", "invalid.md"), content, 0o600))
				return "invalid.md", secret
			},
			want: "is not valid UTF-8",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := writeGraphFixture(t, map[string]string{
				"go.mod": "module example.com/pixiv\n\ngo 1.26.3\n",
				"a/a.go": "package a\n",
			})
			articlePath, secret := test.setup(t, root)
			docsGraphPath := filepath.Join(root, "docs", ".understand-anything", "knowledge-graph.json")
			writeJSONFile(t, docsGraphPath, map[string]any{
				"version": "1.0.0", "project": map[string]any{"name": "fixture"},
				"nodes": []any{map[string]any{
					"id": "article:unsafe", "type": "article", "name": "Unsafe", "filePath": articlePath,
					"summary": "fixture", "tags": []any{"documentation"}, "complexity": "simple",
					"knowledgeMeta": map[string]any{"content": "stale"},
				}},
				"edges": []any{}, "layers": []any{}, "tour": []any{},
			})
			paths := []string{
				filepath.Join(root, ".understand-anything", "intermediate", "scan-result.json"),
				filepath.Join(root, ".understand-anything", "knowledge-graph.json"),
				filepath.Join(root, ".understand-anything", "fingerprints.json"),
				docsGraphPath,
			}
			before := make(map[string][]byte, len(paths))
			for _, path := range paths {
				before[path] = readFile(t, path)
			}

			err := Normalize(root)
			require.ErrorContains(t, err, test.want)
			require.NotContains(t, err.Error(), secret)
			for _, path := range paths {
				require.Equal(t, before[path], readFile(t, path), "failed normalization changed %s", path)
			}
		})
	}
}
