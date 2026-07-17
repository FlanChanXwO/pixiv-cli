package download

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type preserveUgoiraSourceError struct {
	err error
}

func (e preserveUgoiraSourceError) Error() string            { return e.err.Error() }
func (e preserveUgoiraSourceError) Unwrap() error            { return e.err }
func (preserveUgoiraSourceError) PreserveReplacementSource() {}

func TestWriteTempAnimationPreservesNewSourceWhenReplacementRecoveryIsUnresolved(t *testing.T) {
	output := filepath.Join(t.TempDir(), "animation.gif")
	require.NoError(t, os.WriteFile(output, []byte("old animation"), 0o600))
	var source string
	replaceCause := errors.New("replacement recovery unresolved")

	err := writeTempAnimationWithReplacer(context.Background(), output, func(path string) error {
		return os.WriteFile(path, []byte("new animation"), 0o600)
	}, func(sourcePath, _ string) error {
		source = sourcePath
		return preserveUgoiraSourceError{err: replaceCause}
	})
	require.ErrorIs(t, err, replaceCause)

	oldBody, readErr := os.ReadFile(output)
	require.NoError(t, readErr)
	assert.Equal(t, "old animation", string(oldBody))
	newBody, readErr := os.ReadFile(source)
	require.NoError(t, readErr)
	assert.Equal(t, "new animation", string(newBody))
}

func TestWriteTempAnimationCleansNewSourceAfterOrdinaryReplacementFailure(t *testing.T) {
	output := filepath.Join(t.TempDir(), "animation.gif")
	require.NoError(t, os.WriteFile(output, []byte("old animation"), 0o600))
	var source string
	replaceCause := errors.New("replacement unchanged")

	err := writeTempAnimationWithReplacer(context.Background(), output, func(path string) error {
		return os.WriteFile(path, []byte("new animation"), 0o600)
	}, func(sourcePath, _ string) error {
		source = sourcePath
		return replaceCause
	})
	require.ErrorIs(t, err, replaceCause)
	_, statErr := os.Stat(source)
	require.ErrorIs(t, statErr, os.ErrNotExist)
	oldBody, readErr := os.ReadFile(output)
	require.NoError(t, readErr)
	assert.Equal(t, "old animation", string(oldBody))
}
