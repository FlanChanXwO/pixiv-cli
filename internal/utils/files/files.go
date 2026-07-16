package files

import (
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/FlanChanXwO/pixiv-cli/internal/common/constants"
)

type privateFileOps struct {
	write       func(*os.File, []byte) (int, error)
	syncFile    func(*os.File) error
	replaceFile func(string, string) (privateFileReplaceOutcome, error)
	syncParent  func(string) error
}

type privateFileReplaceOutcome struct {
	committed      bool
	preserveSource bool
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
		replaceFile: replacePrivateFile,
		syncParent:  syncParentDirectory,
	}
}

func UserConfigSubdir(appName string) (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, appName), nil
}

func UserConfigFile(appName, filename string) (string, error) {
	dir, err := UserConfigSubdir(appName)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, filename), nil
}

func WritePrivateFile(path string, body []byte, mode os.FileMode) error {
	return writePrivateFile(path, body, mode, defaultPrivateFileOps())
}

// writePrivateFile 在目标同目录完成 staging；普通替换前失败保留旧文件。若平台替换
// 部分完成且恢复也失败，则保留新旧 recovery artifacts，不能由 defer 误删 source。
// 已提交后的 cleanup/durability 失败返回真实错误，但不会伪装成已经回滚。
func writePrivateFile(path string, body []byte, mode os.FileMode, ops privateFileOps) (resultErr error) {
	if err := os.MkdirAll(filepath.Dir(path), constants.PrivateDirMode); err != nil {
		return err
	}
	if err := os.Chmod(filepath.Dir(path), constants.PrivateDirMode); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".pixiv-private-*")
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
	replaced = replacement.committed
	preserveTemporary = replacement.preserveSource
	if !replacement.committed {
		return replaceErr
	}
	return errors.Join(replaceErr, ops.syncParent(filepath.Dir(path)))
}
