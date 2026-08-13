package replace_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/storage/file/replace"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReplaceFileReplacesTargetAndRemovesSource(t *testing.T) {
	directory := t.TempDir()
	source := filepath.Join(directory, "source.tmp")
	target := filepath.Join(directory, "target.txt")
	require.NoError(t, os.WriteFile(source, []byte("new"), 0o600))
	require.NoError(t, os.WriteFile(target, []byte("old"), 0o600))

	require.NoError(t, replace.ReplaceFile(source, target))
	body, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, "new", string(body))
	_, err = os.Stat(source)
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestReplaceFileReportsMissingSource(t *testing.T) {
	directory := t.TempDir()
	err := replace.ReplaceFile(filepath.Join(directory, "missing"), filepath.Join(directory, "target"))
	require.Error(t, err)
	assert.True(t, errors.Is(err, os.ErrNotExist))
}

func TestReplaceDelegatesToReplaceFile(t *testing.T) {
	directory := t.TempDir()
	source := filepath.Join(directory, "source")
	target := filepath.Join(directory, "target")
	require.NoError(t, os.WriteFile(source, []byte("new"), 0o600))
	require.NoError(t, replace.Replace(source, target))
	body, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, "new", string(body))
}
