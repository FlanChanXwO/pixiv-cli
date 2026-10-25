//go:build windows

package atomicfile

import (
	"errors"
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
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

var replaceFileW = windows.NewLazySystemDLL("kernel32.dll").NewProc("ReplaceFileW")

// ReplaceFile 用源文件替换目标路径。临时源文件必须已完整写入并关闭；Windows 在目标存在时
// 必须使用 ReplaceFileW 这一单次替换原语，不能把 MoveFileEx(REPLACE_EXISTING) 当作等价物。
// ReplaceFileW 不支持 WRITE_THROUGH，故不能擅自传入该标志；同步由调用方在替换前完成。
// 同目录 disposable backup 用于恢复 API 部分完成后已移动的旧目标。
func ReplaceFile(sourcePath, targetPath string) error {
	_, err := replaceWithDisposableBackup(sourcePath, targetPath, sourcePath+".recovery")
	return err
}

// ReplaceFileWithBackup 用源文件替换目标，并把旧目标交给 backupPath 延迟清理。运行中的
// Windows 可执行文件无法由自身删除，所以调用方应在下次启动前清理该 .old 文件。若 API
// 报告旧目标已移动到 backup，会先尝试恢复 target；恢复失败则保留 backup 与 source。
func ReplaceFileWithBackup(sourcePath, targetPath, backupPath string) error {
	if backupPath == "" {
		return ReplaceFile(sourcePath, targetPath)
	}
	attempt := replaceFileWithBackupAttempt(sourcePath, targetPath, backupPath)
	_, err := recoverReplacementAttempt(attempt, backupPath, targetPath)
	return err
}

func replacePrivateFile(sourcePath, targetPath string) (Result, error) {
	return replaceWithDisposableBackup(sourcePath, targetPath, sourcePath+".recovery")
}

func replaceFileWithBackupAttempt(sourcePath, targetPath, backupPath string) replacementAttempt {
	from, err := windows.UTF16PtrFromString(sourcePath)
	if err != nil {
		return replacementAttempt{state: replacementUnchanged, err: err}
	}
	to, err := windows.UTF16PtrFromString(targetPath)
	if err != nil {
		return replacementAttempt{state: replacementUnchanged, err: err}
	}
	if _, err := os.Lstat(targetPath); err != nil {
		if !os.IsNotExist(err) {
			return replacementAttempt{state: replacementUnchanged, err: err}
		}
		// 仅在目标确实不存在时创建；若并发创建则由 MoveFileEx 原样报告冲突。
		err = windows.MoveFileEx(from, to, 0)
		return replacementAttempt{state: replacementStateForMove(err), err: err}
	}
	if err := replaceFileW.Find(); err != nil {
		return replacementAttempt{state: replacementUnchanged, err: err}
	}
	backup, err := windows.UTF16PtrFromString(backupPath)
	if err != nil {
		return replacementAttempt{state: replacementUnchanged, err: err}
	}
	success, _, lastErr := replaceFileW.Call(
		uintptr(unsafe.Pointer(to)),
		uintptr(unsafe.Pointer(from)),
		uintptr(unsafe.Pointer(backup)),
		0,
		0,
		0,
	)
	if success == 0 {
		return replacementAttempt{state: replacementStateForWindowsError(lastErr), err: lastErr}
	}
	return replacementAttempt{state: replacementCommitted, backupCreated: true}
}

func replacementStateForMove(err error) replacementAttemptState {
	if err == nil {
		return replacementCommitted
	}
	return replacementUnchanged
}

func replacementStateForWindowsError(err error) replacementAttemptState {
	switch {
	case errors.Is(err, windows.ERROR_UNABLE_TO_MOVE_REPLACEMENT_2):
		// 1177：replacement/source 仍在原名，旧 target 已移动到 backup。
		return replacementOldMovedToBackup
	default:
		// 1176 在指定 backup 时保持 target/source 原名；1175 与其他错误也未提交替换。
		return replacementUnchanged
	}
}

func restoreWindowsBackup(backupPath, targetPath string) error {
	backup, err := windows.UTF16PtrFromString(backupPath)
	if err != nil {
		return err
	}
	target, err := windows.UTF16PtrFromString(targetPath)
	if err != nil {
		return err
	}
	// 1177 状态下 target 应不存在；禁止覆盖可显露并发写入或状态偏差，并保留 backup。
	return windows.MoveFileEx(backup, target, 0)
}

// replaceWithDisposableBackup 使用只服务于本次替换的唯一 backup。若旧目标已被移动，
// 必须先恢复或保留新旧两份恢复材料，不能让调用方按普通失败删除 source。
func replaceWithDisposableBackup(sourcePath, targetPath, backupPath string) (Result, error) {
	attempt := replaceFileWithBackupAttempt(sourcePath, targetPath, backupPath)
	outcome, replaceErr := recoverReplacementAttempt(attempt, backupPath, targetPath)
	if !outcome.Committed || !attempt.backupCreated {
		return outcome, replaceErr
	}
	cleanupErr := os.Remove(backupPath)
	if cleanupErr != nil {
		cleanupErr = fmt.Errorf("remove committed replacement backup: %w", cleanupErr)
	}
	return outcome, errors.Join(replaceErr, cleanupErr)
}

func recoverReplacementAttempt(attempt replacementAttempt, backupPath, targetPath string) (Result, error) {
	switch attempt.state {
	case replacementCommitted:
		return Result{Committed: true}, attempt.err
	case replacementOldMovedToBackup:
		if restoreErr := restoreWindowsBackup(backupPath, targetPath); restoreErr != nil {
			return Result{PreserveSource: true}, markReplacementSourceForPreservation(errors.Join(
				attempt.err,
				fmt.Errorf("restore replaced target from backup: %w", restoreErr),
			))
		}
		return Result{}, attempt.err
	default:
		return Result{}, attempt.err
	}
}
