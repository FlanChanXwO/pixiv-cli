//go:build darwin || linux

package understandgraph

import (
	"os"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeRejectsGoSourceSwapToFIFOBeforeWritingArtifacts(t *testing.T) {
	root := writeGraphFixture(t, map[string]string{
		"go.mod": "module example.com/pixiv\n\ngo 1.26.3\n",
		"a/a.go": "package a\n",
	})
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
			return syscall.Mkfifo(resolvedPath, 0o600)
		})
	}

	err := normalizeWithContainedFileReader(root, reader)
	require.True(t, swapped)
	require.ErrorContains(t, err, "not a regular file")
	for _, artifact := range artifacts {
		require.Equal(t, before[artifact], readFile(t, artifact), "failed normalization changed %s", artifact)
	}
}
