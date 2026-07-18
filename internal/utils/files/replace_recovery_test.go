package files

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/common/constants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnresolvedReplacementRecoveryContractSurvivesWrapping(t *testing.T) {
	replaceErr := errors.New("replacement moved old target")
	restoreErr := errors.New("restore failed")
	_, err := recoverReplacementAttempt(replacementAttempt{
		state: replacementOldMovedToBackup,
		err:   replaceErr,
	}, "backup", "target", func(string, string) error {
		return restoreErr
	})
	wrapped := fmt.Errorf("caller context: %w", err)

	assert.True(t, MustPreserveReplacementSource(wrapped))
	require.ErrorIs(t, wrapped, replaceErr)
	require.ErrorIs(t, wrapped, restoreErr)
}

func TestWritePrivateFileRestoresOldTargetAfterReplacementMovesItToBackup(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "auth.json")
	require.NoError(t, os.WriteFile(target, []byte("old"), constants.PrivateFileMode))
	replaceErr := errors.New("replacement moved old target")
	ops := defaultPrivateFileOps()
	ops.replaceFile = func(source, target string) (privateFileReplaceOutcome, error) {
		backup := source + ".recovery"
		return replaceWithDisposableBackup(source, target, backup, replacementRecoveryOps{
			replace: func(_, target, backup string) replacementAttempt {
				require.NoError(t, os.Rename(target, backup))
				return replacementAttempt{state: replacementOldMovedToBackup, err: replaceErr}
			},
			restore:      os.Rename,
			removeBackup: os.Remove,
		})
	}

	err := writePrivateFile(target, []byte("new"), constants.PrivateFileMode, ops)
	require.ErrorIs(t, err, replaceErr)
	body, readErr := os.ReadFile(target)
	require.NoError(t, readErr)
	assert.Equal(t, "old", string(body))
	assertNoPrivateFileArtifacts(t, dir)
}

func TestWritePrivateFilePreservesOldBackupAndNewSourceWhenRestoreFails(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "auth.json")
	require.NoError(t, os.WriteFile(target, []byte("old"), constants.PrivateFileMode))
	replaceErr := errors.New("replacement moved old target")
	restoreErr := errors.New("restore failed")
	ops := defaultPrivateFileOps()
	ops.replaceFile = func(source, target string) (privateFileReplaceOutcome, error) {
		backup := source + ".recovery"
		return replaceWithDisposableBackup(source, target, backup, replacementRecoveryOps{
			replace: func(_, target, backup string) replacementAttempt {
				require.NoError(t, os.Rename(target, backup))
				return replacementAttempt{state: replacementOldMovedToBackup, err: replaceErr}
			},
			restore: func(string, string) error {
				return restoreErr
			},
			removeBackup: os.Remove,
		})
	}

	err := writePrivateFile(target, []byte("new"), constants.PrivateFileMode, ops)
	require.ErrorIs(t, err, replaceErr)
	require.ErrorIs(t, err, restoreErr)
	_, statErr := os.Stat(target)
	require.ErrorIs(t, statErr, os.ErrNotExist)
	assertRecoveryArtifactBodies(t, dir, "old", "new")
}

func TestWritePrivateFileCleansNewSourceWhenReplacementLeavesNamesUnchanged(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "auth.json")
	require.NoError(t, os.WriteFile(target, []byte("old"), constants.PrivateFileMode))
	replaceErr := errors.New("replacement kept original names")
	ops := defaultPrivateFileOps()
	ops.replaceFile = func(source, target string) (privateFileReplaceOutcome, error) {
		return replaceWithDisposableBackup(source, target, source+".recovery", replacementRecoveryOps{
			replace: func(string, string, string) replacementAttempt {
				return replacementAttempt{state: replacementUnchanged, err: replaceErr}
			},
			restore:      os.Rename,
			removeBackup: os.Remove,
		})
	}

	err := writePrivateFile(target, []byte("new"), constants.PrivateFileMode, ops)
	require.ErrorIs(t, err, replaceErr)
	body, readErr := os.ReadFile(target)
	require.NoError(t, readErr)
	assert.Equal(t, "old", string(body))
	assertNoPrivateFileArtifacts(t, dir)
}

func TestWritePrivateFileCleansNewSourceWhenFirstCreateReplacementFails(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "config.toml")
	replaceErr := errors.New("first create move failed")
	ops := defaultPrivateFileOps()
	ops.replaceFile = func(string, string) (privateFileReplaceOutcome, error) {
		return privateFileReplaceOutcome{}, replaceErr
	}

	err := writePrivateFile(target, []byte("new"), constants.PrivateFileMode, ops)
	require.ErrorIs(t, err, replaceErr)
	assert.Equal(t, WriteCommitOutcomeNotCommitted, PrivateFileWriteCommitOutcome(err))
	_, statErr := os.Stat(target)
	require.ErrorIs(t, statErr, os.ErrNotExist)
	assertNoPrivateFileArtifacts(t, dir)
}

func TestWritePrivateFileRunsParentSyncAfterCommittedBackupCleanupFails(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "auth.json")
	require.NoError(t, os.WriteFile(target, []byte("old"), constants.PrivateFileMode))
	cleanupErr := errors.New("backup cleanup failed")
	parentSyncErr := errors.New("parent sync failed")
	parentSyncCalled := false
	ops := defaultPrivateFileOps()
	ops.replaceFile = func(source, target string) (privateFileReplaceOutcome, error) {
		return replaceWithDisposableBackup(source, target, source+".recovery", replacementRecoveryOps{
			replace: func(source, target, backup string) replacementAttempt {
				require.NoError(t, os.Rename(target, backup))
				require.NoError(t, os.Rename(source, target))
				return replacementAttempt{state: replacementCommitted, backupCreated: true}
			},
			restore: os.Rename,
			removeBackup: func(string) error {
				return cleanupErr
			},
		})
	}
	ops.syncParent = func(string) error {
		parentSyncCalled = true
		return parentSyncErr
	}

	err := writePrivateFile(target, []byte("new"), constants.PrivateFileMode, ops)
	require.ErrorIs(t, err, cleanupErr)
	require.ErrorIs(t, err, parentSyncErr)
	assert.True(t, parentSyncCalled)
	body, readErr := os.ReadFile(target)
	require.NoError(t, readErr)
	assert.Equal(t, "new", string(body))
	assertRecoveryArtifactBodies(t, dir, "old")
}

func assertRecoveryArtifactBodies(t *testing.T, dir string, expected ...string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, ".pixiv-private-*"))
	require.NoError(t, err)
	require.Len(t, matches, len(expected))
	bodies := make([]string, 0, len(matches))
	for _, match := range matches {
		body, readErr := os.ReadFile(match)
		require.NoError(t, readErr)
		bodies = append(bodies, string(body))
	}
	assert.ElementsMatch(t, expected, bodies)
}
