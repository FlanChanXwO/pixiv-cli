package files

import (
	"errors"
	"fmt"
)

type replacementAttemptState uint8

const (
	replacementUnchanged replacementAttemptState = iota
	replacementCommitted
	replacementOldMovedToBackup
)

type replacementAttempt struct {
	state         replacementAttemptState
	backupCreated bool
	err           error
}

type replacementRecoveryOps struct {
	replace      func(sourcePath, targetPath, backupPath string) replacementAttempt
	restore      func(backupPath, targetPath string) error
	removeBackup func(backupPath string) error
}

// replaceWithDisposableBackup 使用只服务于本次替换的唯一 backup。若旧目标已被移动，
// 必须先恢复或保留新旧两份恢复材料，不能让调用方按普通失败删除 source。
func replaceWithDisposableBackup(sourcePath, targetPath, backupPath string, ops replacementRecoveryOps) (privateFileReplaceOutcome, error) {
	attempt := ops.replace(sourcePath, targetPath, backupPath)
	outcome, replaceErr := recoverReplacementAttempt(attempt, backupPath, targetPath, ops.restore)
	if !outcome.committed || !attempt.backupCreated {
		return outcome, replaceErr
	}
	cleanupErr := ops.removeBackup(backupPath)
	if cleanupErr != nil {
		cleanupErr = fmt.Errorf("remove committed replacement backup: %w", cleanupErr)
	}
	return outcome, errors.Join(replaceErr, cleanupErr)
}

func recoverReplacementAttempt(attempt replacementAttempt, backupPath, targetPath string, restore func(string, string) error) (privateFileReplaceOutcome, error) {
	switch attempt.state {
	case replacementCommitted:
		return privateFileReplaceOutcome{committed: true}, attempt.err
	case replacementOldMovedToBackup:
		if restoreErr := restore(backupPath, targetPath); restoreErr != nil {
			return privateFileReplaceOutcome{preserveSource: true}, errors.Join(
				attempt.err,
				fmt.Errorf("restore replaced target from backup: %w", restoreErr),
			)
		}
		return privateFileReplaceOutcome{}, attempt.err
	default:
		return privateFileReplaceOutcome{}, attempt.err
	}
}
