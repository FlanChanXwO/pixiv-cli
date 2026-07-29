package files

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	constants "github.com/FlanChanXwO/pixiv-cli/internal/platform/localstate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWritePrivateFileKeepsOldFileWhenWriteFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	require.NoError(t, os.WriteFile(path, []byte("old"), constants.PrivateFileMode))
	ops := defaultPrivateFileOps()
	ops.write = func(*os.File, []byte) (int, error) {
		return 0, errors.New("write failed")
	}

	err := writePrivateFile(path, []byte("new"), constants.PrivateFileMode, ops)
	require.ErrorContains(t, err, "write failed")

	body, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.Equal(t, "old", string(body))
	assertNoPrivateFileTemporaries(t, dir)
}

func TestWritePrivateFileKeepsOldFileWhenFileSyncFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "auth.json")
	require.NoError(t, os.WriteFile(path, []byte("old"), constants.PrivateFileMode))
	ops := defaultPrivateFileOps()
	ops.syncFile = func(*os.File) error {
		return errors.New("file sync failed")
	}

	err := writePrivateFile(path, []byte("new"), constants.PrivateFileMode, ops)
	require.ErrorContains(t, err, "file sync failed")

	body, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.Equal(t, "old", string(body))
	assertNoPrivateFileTemporaries(t, dir)
}

func TestWritePrivateFileKeepsOldFileWhenReplaceFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	require.NoError(t, os.WriteFile(path, []byte("old"), constants.PrivateFileMode))
	ops := defaultPrivateFileOps()
	ops.replaceFile = func(string, string) (privateFileReplaceOutcome, error) {
		return privateFileReplaceOutcome{}, errors.New("replace failed")
	}

	err := writePrivateFile(path, []byte("new"), constants.PrivateFileMode, ops)
	require.ErrorContains(t, err, "replace failed")

	body, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.Equal(t, "old", string(body))
	assertNoPrivateFileTemporaries(t, dir)
}

func TestWritePrivateFileRejectsPartialWriteWithoutReplacingOldFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	require.NoError(t, os.WriteFile(path, []byte("old"), constants.PrivateFileMode))
	ops := defaultPrivateFileOps()
	ops.write = func(file *os.File, body []byte) (int, error) {
		return file.Write(body[:len(body)-1])
	}

	err := writePrivateFile(path, []byte("new"), constants.PrivateFileMode, ops)
	require.ErrorIs(t, err, io.ErrShortWrite)

	body, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.Equal(t, "old", string(body))
	assertNoPrivateFileTemporaries(t, dir)
}

func TestUserDataFileBuildsPathDirectlyUnderUserHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	path, err := UserDataFile(constants.AppDataDirName, "auth.json")
	require.NoError(t, err)

	assert.Equal(t, filepath.Join(home, constants.AppDataDirName, "auth.json"), path)
}

func TestWritePrivateFileCreatesParentAndFileWithPlatformPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pixiv", "config.toml")

	require.NoError(t, WritePrivateFile(path, []byte("body"), constants.PrivateFileMode))

	parent, err := os.Stat(filepath.Dir(path))
	require.NoError(t, err)
	assertPlatformDirMode(t, parent.Mode().Perm())

	info, err := os.Stat(path)
	require.NoError(t, err)
	assertPlatformFileMode(t, info.Mode().Perm())
	body, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "body", string(body))
	assertNoPrivateFileTemporaries(t, filepath.Dir(path))
}

func TestWritePrivateFileAppliesPlatformModeToExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pixiv", "auth.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), constants.PrivateDirMode))
	require.NoError(t, os.WriteFile(path, []byte("old"), 0o644))

	require.NoError(t, WritePrivateFile(path, []byte("new"), constants.PrivateFileMode))

	info, err := os.Stat(path)
	require.NoError(t, err)
	assertPlatformFileMode(t, info.Mode().Perm())
	body, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "new", string(body))
	assertNoPrivateFileTemporaries(t, filepath.Dir(path))
}

func TestWritePrivateFileAppliesPlatformModeToExistingParentDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pixiv", "auth.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))

	require.NoError(t, WritePrivateFile(path, []byte("body"), constants.PrivateFileMode))

	parent, err := os.Stat(filepath.Dir(path))
	require.NoError(t, err)
	assertPlatformDirMode(t, parent.Mode().Perm())
}

func TestWriteSecretFileCreatesPrivateFileWithoutChangingExistingParentOrTarget(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows ACL 由 build-tagged 测试断言")
	}
	directory := filepath.Join(t.TempDir(), "exports")
	require.NoError(t, os.Mkdir(directory, 0o751))
	path := filepath.Join(directory, "auth-export.json")

	require.NoError(t, WriteSecretFile(path, []byte("first-secret"), false))
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	parent, err := os.Stat(directory)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o751), parent.Mode().Perm())

	err = WriteSecretFile(path, []byte("second-secret"), false)
	require.ErrorIs(t, err, os.ErrExist)
	var typed *SecretFileWriteError
	require.ErrorAs(t, err, &typed)
	assert.Equal(t, WriteCommitOutcomeNotCommitted, typed.CommitOutcome())
	assert.Contains(t, err.Error(), "destination already exists")
	assert.NotContains(t, err.Error(), path)
	body, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.Equal(t, "first-secret", string(body))
}

func TestWriteSecretFileRemovesIncompleteExclusiveDestination(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "auth-export.json")
	ops := defaultSecretFileOps()
	ops.write = func(file *os.File, body []byte) (int, error) {
		written, err := file.Write(body[:len(body)-1])
		return written, errors.Join(err, errors.New("synthetic write failure"))
	}

	err := writeSecretFile(path, []byte("secret-body"), false, ops)
	require.Error(t, err)
	assert.Equal(t, WriteCommitOutcomeNotCommitted, SecretFileWriteCommitOutcome(err))
	_, statErr := os.Stat(path)
	assert.ErrorIs(t, statErr, os.ErrNotExist)
	matches, globErr := filepath.Glob(filepath.Join(directory, ".pixiv-secret-*"))
	require.NoError(t, globErr)
	assert.Empty(t, matches)
}

func TestWriteSecretFileExclusiveCreateHasSingleConcurrentWinner(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth-export.json")
	const writers = 8
	start := make(chan struct{})
	results := make(chan struct {
		body string
		err  error
	}, writers)
	var group sync.WaitGroup
	for index := 0; index < writers; index++ {
		body := fmt.Sprintf("complete-secret-%d", index)
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			results <- struct {
				body string
				err  error
			}{body: body, err: WriteSecretFile(path, []byte(body), false)}
		}()
	}
	close(start)
	group.Wait()
	close(results)

	winners := []string{}
	for result := range results {
		if result.err == nil {
			winners = append(winners, result.body)
			continue
		}
		assert.ErrorIs(t, result.err, os.ErrExist)
		assert.Equal(t, WriteCommitOutcomeNotCommitted, SecretFileWriteCommitOutcome(result.err))
	}
	require.Len(t, winners, 1)
	body, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, winners[0], string(body))
}

func TestWriteSecretFileForceReportsReplacementCommitOutcome(t *testing.T) {
	for _, test := range []struct {
		name        string
		replace     func(string, string) (privateFileReplaceOutcome, error)
		wantOutcome WriteCommitOutcome
		wantBody    string
		wantTemps   int
	}{
		{
			name: "not committed",
			replace: func(string, string) (privateFileReplaceOutcome, error) {
				return privateFileReplaceOutcome{}, errors.New("replacement failed")
			},
			wantOutcome: WriteCommitOutcomeNotCommitted,
			wantBody:    "old",
		},
		{
			name: "committed with durability error",
			replace: func(source, target string) (privateFileReplaceOutcome, error) {
				require.NoError(t, os.Rename(source, target))
				return privateFileReplaceOutcome{committed: true}, errors.New("post-commit failure")
			},
			wantOutcome: WriteCommitOutcomeCommitted,
			wantBody:    "new-secret-canary",
		},
		{
			name: "unknown preserves recovery source",
			replace: func(string, string) (privateFileReplaceOutcome, error) {
				return privateFileReplaceOutcome{preserveSource: true}, errors.New("replacement state unknown")
			},
			wantOutcome: WriteCommitOutcomeUnknown,
			wantBody:    "old",
			wantTemps:   1,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			path := filepath.Join(directory, "destination-secret-path.json")
			require.NoError(t, os.WriteFile(path, []byte("old"), 0o600))
			ops := defaultSecretFileOps()
			ops.replace = test.replace

			err := writeSecretFile(path, []byte("new-secret-canary"), true, ops)
			require.Error(t, err)
			assert.Equal(t, test.wantOutcome, SecretFileWriteCommitOutcome(err))
			assert.NotContains(t, err.Error(), path)
			assert.NotContains(t, err.Error(), "new-secret-canary")
			body, readErr := os.ReadFile(path)
			require.NoError(t, readErr)
			assert.Equal(t, test.wantBody, string(body))
			matches, globErr := filepath.Glob(filepath.Join(directory, ".pixiv-secret-*"))
			require.NoError(t, globErr)
			assert.Len(t, matches, test.wantTemps)
		})
	}
}

func assertPlatformFileMode(t *testing.T, actual os.FileMode) {
	t.Helper()
	if runtime.GOOS == "windows" {
		// Go 的 Windows mode bits 不能证明 DACL 私有性；本任务不声称主动收紧 DACL。
		return
	}
	assert.Equal(t, os.FileMode(constants.PrivateFileMode), actual)
}

func assertPlatformDirMode(t *testing.T, actual os.FileMode) {
	t.Helper()
	if runtime.GOOS == "windows" {
		// Mkdir mode 在 Windows 被忽略，不能把 0777 观测值当作 ACL 保证。
		return
	}
	assert.Equal(t, os.FileMode(constants.PrivateDirMode), actual)
}

func assertNoPrivateFileTemporaries(t *testing.T, dir string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, ".pixiv-private-*"))
	require.NoError(t, err)
	assert.Empty(t, matches)
}

func assertNoPrivateFileArtifacts(t *testing.T, dir string) {
	t.Helper()
	assertNoPrivateFileTemporaries(t, dir)
	matches, err := filepath.Glob(filepath.Join(dir, ".pixiv-private-*.recovery"))
	require.NoError(t, err)
	assert.Empty(t, matches)
}

func TestReplaceFileReplacesTargetAndRemovesSource(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.tmp")
	target := filepath.Join(dir, "target.txt")
	require.NoError(t, os.WriteFile(source, []byte("new"), 0o644))
	require.NoError(t, os.WriteFile(target, []byte("old"), 0o644))

	require.NoError(t, ReplaceFile(source, target))

	body, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, "new", string(body))
	_, err = os.Stat(source)
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(source + ".recovery")
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestReplaceFileCreatesMissingTargetAndRemovesSource(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.tmp")
	target := filepath.Join(dir, "target.txt")
	require.NoError(t, os.WriteFile(source, []byte("new"), 0o644))

	require.NoError(t, ReplaceFile(source, target))

	body, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, "new", string(body))
	_, err = os.Stat(source)
	require.ErrorIs(t, err, os.ErrNotExist)
}
