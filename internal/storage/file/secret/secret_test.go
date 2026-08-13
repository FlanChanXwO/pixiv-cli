package secret_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/platform/localstate"
	secret "github.com/FlanChanXwO/pixiv-cli/internal/storage/file/secret"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWritePrivateFileCreatesPrivatePathAndReplacesExistingContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.toml")
	require.NoError(t, secret.WritePrivateFile(path, []byte("old"), localstate.PrivateFileMode))
	require.NoError(t, secret.WritePrivateFile(path, []byte("new"), localstate.PrivateFileMode))

	body, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "new", string(body))
	assertPrivateFileMode(t, path)
	assertPrivateDirMode(t, filepath.Dir(path))
	assertNoPrivateTemporary(t, filepath.Dir(path))
}

func TestWritePrivateFileReportsReplacementFailureWithoutHidingExistingDirectory(t *testing.T) {
	directory := t.TempDir()
	target := filepath.Join(directory, "target-directory")
	require.NoError(t, os.Mkdir(target, localstate.PrivateDirMode))

	err := secret.WritePrivateFile(target, []byte("new"), localstate.PrivateFileMode)
	require.Error(t, err)
	info, statErr := os.Stat(target)
	require.NoError(t, statErr)
	assert.True(t, info.IsDir())
	assertNoPrivateTemporary(t, directory)
}

func TestEnsurePrivateFileDoesNotOverwriteExistingContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "state.json")
	require.NoError(t, secret.EnsurePrivateFile(path, []byte("first"), localstate.PrivateFileMode))
	require.NoError(t, secret.EnsurePrivateFile(path, []byte("second"), localstate.PrivateFileMode))

	body, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "first", string(body))
	assertPrivateFileMode(t, path)
}

func TestWriteSecretFileRedactsPathAndPreservesExistingDestination(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth-export.json")
	require.NoError(t, secret.WriteSecretFile(path, []byte("first-secret"), false))

	err := secret.WriteSecretFile(path, []byte("second-secret"), false)
	require.ErrorIs(t, err, os.ErrExist)
	assert.NotContains(t, err.Error(), path)
	assert.NotContains(t, err.Error(), "second-secret")
	assert.Equal(t, secret.WriteCommitOutcomeNotCommitted, secret.SecretFileWriteCommitOutcome(err))

	body, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.Equal(t, "first-secret", string(body))
}

func TestWriteSecretFileForceReplacesDestination(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth-export.json")
	require.NoError(t, secret.WriteSecretFile(path, []byte("old"), false))
	require.NoError(t, secret.WriteSecretFile(path, []byte("new"), true))

	body, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "new", string(body))
}

func TestWriteJSONAndReadJSONUsePrivateFileProtocol(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	want := map[string]any{"user_id": float64(42), "name": "alice"}
	require.NoError(t, secret.WriteJSON(path, want))

	var got map[string]any
	present, err := secret.ReadJSON(path, &got)
	require.NoError(t, err)
	assert.True(t, present)
	assert.Equal(t, want, got)

	var missing map[string]any
	present, err = secret.ReadJSON(filepath.Join(t.TempDir(), "missing.json"), &missing)
	require.NoError(t, err)
	assert.False(t, present)

	require.NoError(t, os.WriteFile(path, []byte("not-json"), localstate.PrivateFileMode))
	_, err = secret.ReadJSON(path, &got)
	assert.Error(t, err)
}

func assertPrivateFileMode(t *testing.T, path string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		return
	}
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(localstate.PrivateFileMode), info.Mode().Perm())
}

func assertPrivateDirMode(t *testing.T, path string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		return
	}
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(localstate.PrivateDirMode), info.Mode().Perm())
}

func assertNoPrivateTemporary(t *testing.T, directory string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(directory, ".pixiv-private-*"))
	require.NoError(t, err)
	assert.Empty(t, matches)
}

func TestSecretFileWriteErrorKeepsCauseClassifiable(t *testing.T) {
	directory := t.TempDir()
	parent := filepath.Join(directory, "not-a-directory")
	require.NoError(t, os.WriteFile(parent, []byte("file"), localstate.PrivateFileMode))
	path := filepath.Join(parent, "auth.json")
	err := secret.WriteSecretFile(path, []byte("secret"), false)
	require.Error(t, err)
	var typed *secret.SecretFileWriteError
	require.ErrorAs(t, err, &typed)
	assert.NotEmpty(t, typed.Error())
}

func TestWriteJSONOutputIsValidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	require.NoError(t, secret.WriteJSON(path, map[string]string{"status": "ok"}))
	body, err := os.ReadFile(path)
	require.NoError(t, err)
	var decoded map[string]string
	require.NoError(t, json.Unmarshal(body, &decoded))
	assert.Equal(t, "ok", decoded["status"])
}
