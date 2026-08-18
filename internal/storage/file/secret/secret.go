package secret

import (
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/FlanChanXwO/pixiv-cli/internal/config/paths"
	filereplace "github.com/FlanChanXwO/pixiv-cli/internal/storage/file/replace"
)

type privateFileOps struct {
	write       func(*os.File, []byte) (int, error)
	syncFile    func(*os.File) error
	replaceFile func(string, string) (filereplace.Result, error)
	syncParent  func(string) error
}

// defaultPrivateFileOps 只为不可稳定复现的文件系统失败提供按调用注入点，
// 生产路径不使用全局可变 hook。
func defaultPrivateFileOps() privateFileOps {
	return privateFileOps{
		write: func(file *os.File, body []byte) (int, error) {
			return file.Write(body)
		},
		syncFile: func(file *os.File) error {
			return file.Sync()
		},
		replaceFile: filereplace.ReplacePrivateFile,
		syncParent:  filereplace.SyncParentDirectory,
	}
}

func WritePrivateFile(path string, body []byte, mode os.FileMode) error {
	return writePrivateFile(path, body, mode, defaultPrivateFileOps())
}

// EnsurePrivateDir 创建目录并把权限收紧到本项目约定的私有目录模式。
// 该 API 不负责清理或覆盖目录之外的内容。
func EnsurePrivateDir(path string) error {
	if err := os.MkdirAll(path, paths.PrivateDirMode); err != nil {
		return err
	}
	return os.Chmod(path, paths.PrivateDirMode)
}

// EnsurePrivateFile 创建一个私密文件但绝不覆盖已有内容。它用于配置基线等
// 首次启动文件；正常更新仍应使用 WritePrivateFile 的原子替换协议。
func EnsurePrivateFile(path string, body []byte, mode os.FileMode) (resultErr error) {
	directory := filepath.Dir(path)
	if err := EnsurePrivateDir(directory); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if errors.Is(err, os.ErrExist) {
		return nil
	}
	if err != nil {
		return err
	}
	complete := false
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			resultErr = errors.Join(resultErr, closeErr)
		}
		if !complete {
			if removeErr := os.Remove(path); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
				resultErr = errors.Join(resultErr, removeErr)
			}
		}
	}()
	if err := file.Chmod(mode); err != nil {
		return err
	}
	written, err := file.Write(body)
	if err != nil {
		return err
	}
	if written != len(body) {
		return io.ErrShortWrite
	}
	if err := file.Sync(); err != nil {
		return err
	}
	complete = true
	return nil
}

// writePrivateFile 在目标同目录完成 staging；普通替换前失败保留旧文件。若平台替换
// 部分完成且恢复也失败，则保留新旧 recovery artifacts，不能由 defer 误删 source。
// 已提交后的 cleanup/durability 失败返回真实错误，但不会伪装成已经回滚。
func writePrivateFile(path string, body []byte, mode os.FileMode, ops privateFileOps) (resultErr error) {
	commitOutcome := WriteCommitOutcomeNotCommitted
	defer func() {
		resultErr = withPrivateFileWriteCommitOutcome(resultErr, commitOutcome)
	}()
	targetDirectory := filepath.Dir(path)
	newDirectories, err := missingDirectoryChain(targetDirectory)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(targetDirectory, paths.PrivateDirMode); err != nil {
		return err
	}
	if err := os.Chmod(targetDirectory, paths.PrivateDirMode); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(targetDirectory, ".pixiv-private-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	closed := false
	replaced := false
	preserveTemporary := false
	defer func() {
		if !closed {
			if err := temporary.Close(); err != nil {
				resultErr = errors.Join(resultErr, err)
			}
		}
		if !replaced && !preserveTemporary {
			if err := os.Remove(temporaryPath); err != nil && !errors.Is(err, os.ErrNotExist) {
				resultErr = errors.Join(resultErr, err)
			}
		}
	}()

	if err := temporary.Chmod(mode); err != nil {
		return err
	}
	written, err := ops.write(temporary, body)
	if err != nil {
		return err
	}
	if written != len(body) {
		return io.ErrShortWrite
	}
	if err := ops.syncFile(temporary); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		closed = true
		return err
	}
	closed = true
	replacement, replaceErr := ops.replaceFile(temporaryPath, path)
	replaced = replacement.Committed
	preserveTemporary = replacement.PreserveSource
	switch {
	case replacement.Committed:
		commitOutcome = WriteCommitOutcomeCommitted
	case replacement.PreserveSource:
		commitOutcome = WriteCommitOutcomeUnknown
	}
	if !replacement.Committed {
		return replaceErr
	}
	return errors.Join(replaceErr, syncCommittedPrivatePath(targetDirectory, newDirectories, ops.syncParent))
}

// missingDirectoryChain 按 leaf→root 记录调用开始时尚不存在的目录。MkdirAll 成功后，
// 这些目录各自的外层 parent entry 都必须在 file replacement 提交后同步。
func missingDirectoryChain(directory string) ([]string, error) {
	var missing []string
	for current := filepath.Clean(directory); ; current = filepath.Dir(current) {
		_, err := os.Stat(current)
		if err == nil {
			return missing, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		missing = append(missing, current)
		parent := filepath.Dir(current)
		if parent == current {
			return missing, nil
		}
	}
}

func syncCommittedPrivatePath(targetDirectory string, newDirectories []string, syncDirectory func(string) error) error {
	errs := []error{syncDirectory(targetDirectory)}
	for _, directory := range newDirectories {
		errs = append(errs, syncDirectory(filepath.Dir(directory)))
	}
	return errors.Join(errs...)
}
