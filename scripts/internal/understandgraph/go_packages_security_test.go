package understandgraph

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeRejectsUnsafeGoSourcesBeforeWritingArtifacts(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*testing.T, string) (string, string, string)
		want  string
	}{
		{
			name: "empty path",
			setup: func(t *testing.T, root string) (string, string, string) {
				return "", `path ""`, "EMPTY_PATH_MUST_NOT_READ_CONTENT"
			},
			want: "is empty",
		},
		{
			name: "absolute path",
			setup: func(t *testing.T, root string) (string, string, string) {
				path := filepath.Join(t.TempDir(), "absolute.go")
				secret := "ABSOLUTE_GO_SOURCE_MUST_NOT_BE_READ"
				require.NoError(t, os.WriteFile(path, []byte("package a\n// "+secret+"\n"), 0o600))
				return path, path, secret
			},
			want: "absolute path outside repository",
		},
		{
			name: "lexical parent traversal",
			setup: func(t *testing.T, root string) (string, string, string) {
				name := filepath.Base(root) + "-outside.go"
				path := filepath.Join(filepath.Dir(root), name)
				secret := "LEXICAL_GO_SOURCE_MUST_NOT_BE_READ"
				require.NoError(t, os.WriteFile(path, []byte("package a\n// "+secret+"\n"), 0o600))
				relative := filepath.Join("..", name)
				return relative, relative, secret
			},
			want: "outside repository",
		},
		{
			name: "symlink escape",
			setup: func(t *testing.T, root string) (string, string, string) {
				outside := filepath.Join(t.TempDir(), "outside.go")
				secret := "SYMLINKED_GO_SOURCE_MUST_NOT_BE_READ"
				// 同步 fixture 摘要，使旧实现确实会跟随链接并成功完成归一化。
				content := "package a\n// " + secret + "\n"
				require.NoError(t, os.WriteFile(outside, []byte(content), 0o600))
				setGoSourceMetadata(t, root, content)
				path := filepath.Join(root, "a", "a.go")
				require.NoError(t, os.Remove(path))
				if err := os.Symlink(outside, path); err != nil {
					if os.IsPermission(err) || errors.Is(err, os.ErrPermission) {
						t.Skipf("test environment does not permit symlink creation: %v", err)
					}
					require.NoError(t, err)
				}
				return "a/a.go", "a/a.go", secret
			},
			want: "resolves outside repository",
		},
		{
			name: "non regular file",
			setup: func(t *testing.T, root string) (string, string, string) {
				path := filepath.Join(root, "a", "directory.go")
				require.NoError(t, os.Mkdir(path, 0o700))
				return "a/directory.go", "a/directory.go", "NON_REGULAR_CONTENT_MUST_NOT_BE_REPORTED"
			},
			want: "not a regular file",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := writeGraphFixture(t, map[string]string{
				"go.mod": "module example.com/pixiv\n\ngo 1.26.3\n",
				"a/a.go": "package a\n",
			})
			path, controlledPath, secret := test.setup(t, root)
			setOnlyGoScanPath(t, root, path)
			artifacts := graphArtifactPaths(root)
			before := snapshotFiles(t, artifacts)

			err := Normalize(root)
			require.ErrorContains(t, err, test.want)
			// scan-result.json 的路径协议固定为 slash；Windows 上错误也必须反映该输入，
			// 不能把宿主文件系统的分隔符当成安全契约的一部分。
			require.ErrorContains(t, err, filepath.ToSlash(controlledPath))
			require.NotContains(t, err.Error(), secret)
			for _, artifact := range artifacts {
				require.Equal(t, before[artifact], readFile(t, artifact), "failed normalization changed %s", artifact)
			}
		})
	}
}

func TestNormalizeAllowsRegularAndInternalSymlinkGoSources(t *testing.T) {
	t.Run("regular file", func(t *testing.T) {
		root := writeGraphFixture(t, map[string]string{
			"go.mod": "module example.com/pixiv\n\ngo 1.26.3\n",
			"a/a.go": "package a\n",
		})
		require.NoError(t, Normalize(root))
	})

	t.Run("symlink remains in repository", func(t *testing.T) {
		root := writeGraphFixture(t, map[string]string{
			"go.mod": "module example.com/pixiv\n\ngo 1.26.3\n",
			"a/a.go": "package a\n",
		})
		target := filepath.Join(root, "internal-source", "a.txt")
		require.NoError(t, os.MkdirAll(filepath.Dir(target), 0o755))
		require.NoError(t, os.WriteFile(target, []byte("package a\n"), 0o600))
		link := filepath.Join(root, "a", "a.go")
		require.NoError(t, os.Remove(link))
		if err := os.Symlink(filepath.Join("..", "internal-source", "a.txt"), link); err != nil {
			if os.IsPermission(err) || errors.Is(err, os.ErrPermission) {
				t.Skipf("test environment does not permit symlink creation: %v", err)
			}
			require.NoError(t, err)
		}

		require.NoError(t, Normalize(root))
	})
}

func TestNormalizeRejectsGoSourceSwapAfterInitialResolutionBeforeWritingArtifacts(t *testing.T) {
	root := writeGraphFixture(t, map[string]string{
		"go.mod": "module example.com/pixiv\n\ngo 1.26.3\n",
		"a/a.go": "package a\n",
	})
	secret := "RACE_SWAP_SECRET_MUST_NOT_BE_READ"
	outside := filepath.Join(t.TempDir(), "outside.go")
	require.NoError(t, os.WriteFile(outside, []byte(secret+"\n"), 0o600))
	artifacts := graphArtifactPaths(root)
	before := snapshotFiles(t, artifacts)

	swapped := false
	reader := func(root, relativePath, boundaryName string) ([]byte, error) {
		return readContainedRegularFileWithHook(root, relativePath, boundaryName, func(resolvedPath string) error {
			if swapped || boundaryName != "repository" {
				return nil
			}
			swapped = true
			if err := os.Remove(resolvedPath); err != nil {
				return err
			}
			return os.Symlink(outside, resolvedPath)
		})
	}

	err := normalizeWithContainedFileReader(root, reader)
	require.True(t, swapped)
	require.ErrorContains(t, err, "resolves outside repository")
	require.NotContains(t, err.Error(), secret)
	for _, artifact := range artifacts {
		require.Equal(t, before[artifact], readFile(t, artifact), "failed normalization changed %s", artifact)
	}
}

func TestNormalizeRejectsGoSourceIdentityChangeBeforeWritingArtifacts(t *testing.T) {
	root := writeGraphFixture(t, map[string]string{
		"go.mod": "module example.com/pixiv\n\ngo 1.26.3\n",
		"a/a.go": "package a\n",
	})
	secret := "IN_REPOSITORY_RACE_SECRET_MUST_NOT_BE_READ"
	replacement := filepath.Join(root, "replacement.go")
	require.NoError(t, os.WriteFile(replacement, []byte(secret+"\n"), 0o600))
	artifacts := graphArtifactPaths(root)
	before := snapshotFiles(t, artifacts)

	swapped := false
	reader := func(root, relativePath, boundaryName string) ([]byte, error) {
		return readContainedRegularFileWithHook(root, relativePath, boundaryName, func(resolvedPath string) error {
			if swapped || boundaryName != "repository" {
				return nil
			}
			swapped = true
			if err := os.Remove(resolvedPath); err != nil {
				return err
			}
			return os.Rename(replacement, resolvedPath)
		})
	}

	err := normalizeWithContainedFileReader(root, reader)
	require.True(t, swapped)
	require.ErrorContains(t, err, "changed after initial validation")
	require.NotContains(t, err.Error(), secret)
	for _, artifact := range artifacts {
		require.Equal(t, before[artifact], readFile(t, artifact), "failed normalization changed %s", artifact)
	}
}

func TestNormalizeRejectsGoSourceContentChangeBeforeWritingArtifacts(t *testing.T) {
	root := writeGraphFixture(t, map[string]string{
		"go.mod": "module example.com/pixiv\n\ngo 1.26.3\n",
		"a/a.go": "package a\n",
	})
	secret := "IN_PLACE_RACE_SECRET_MUST_NOT_BE_READ"
	artifacts := graphArtifactPaths(root)
	before := snapshotFiles(t, artifacts)

	changed := false
	reader := func(root, relativePath, boundaryName string) ([]byte, error) {
		return readContainedRegularFileWithHook(root, relativePath, boundaryName, func(resolvedPath string) error {
			if changed || boundaryName != "repository" {
				return nil
			}
			changed = true
			return os.WriteFile(resolvedPath, []byte(secret+"\n"), 0o600)
		})
	}

	err := normalizeWithContainedFileReader(root, reader)
	require.True(t, changed)
	require.ErrorContains(t, err, "changed after initial validation")
	require.NotContains(t, err.Error(), secret)
	for _, artifact := range artifacts {
		require.Equal(t, before[artifact], readFile(t, artifact), "failed normalization changed %s", artifact)
	}
}

func setOnlyGoScanPath(t *testing.T, root, path string) {
	t.Helper()
	scanPath := filepath.Join(root, ".understand-anything", "intermediate", "scan-result.json")
	var scan map[string]any
	readJSONFile(t, scanPath, &scan)
	file := scan["files"].([]any)[0].(map[string]any)
	file["path"] = filepath.ToSlash(path)
	writeJSONFile(t, scanPath, scan)
}

func setGoSourceMetadata(t *testing.T, root, content string) {
	t.Helper()
	scanPath := filepath.Join(root, ".understand-anything", "intermediate", "scan-result.json")
	var scan map[string]any
	readJSONFile(t, scanPath, &scan)
	scan["files"].([]any)[0].(map[string]any)["sizeLines"] = strings.Count(content, "\n")
	writeJSONFile(t, scanPath, scan)

	fingerprintPath := filepath.Join(root, ".understand-anything", "fingerprints.json")
	var fingerprints map[string]any
	readJSONFile(t, fingerprintPath, &fingerprints)
	fingerprint := fingerprints["files"].(map[string]any)["a/a.go"].(map[string]any)
	fingerprint["contentHash"] = fixtureContentHash(content)
	fingerprint["totalLines"] = strings.Count(content, "\n") + 1
	writeJSONFile(t, fingerprintPath, fingerprints)
}

func graphArtifactPaths(root string) []string {
	return []string{
		filepath.Join(root, ".understand-anything", "intermediate", "scan-result.json"),
		filepath.Join(root, ".understand-anything", "knowledge-graph.json"),
		filepath.Join(root, ".understand-anything", "fingerprints.json"),
		filepath.Join(root, "docs", ".understand-anything", "knowledge-graph.json"),
	}
}

func snapshotFiles(t *testing.T, paths []string) map[string][]byte {
	t.Helper()
	snapshot := make(map[string][]byte, len(paths))
	for _, path := range paths {
		snapshot[path] = readFile(t, path)
	}
	return snapshot
}
