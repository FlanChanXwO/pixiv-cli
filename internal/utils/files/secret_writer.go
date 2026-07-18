package files

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
)

// SecretFileWriteError 隐藏任意目标路径及底层可能携带的路径，同时保留 errors.Is/As。
type SecretFileWriteError struct {
	outcome WriteCommitOutcome
	cause   error
}

type secretFileOps struct {
	openExclusive func(string) (*os.File, error)
	protect       func(*os.File) error
	write         func(*os.File, []byte) (int, error)
	sync          func(*os.File) error
	close         func(*os.File) error
	remove        func(string) error
	replace       func(string, string) (privateFileReplaceOutcome, error)
	protectPath   func(string) error
	syncParent    func(string) error
}

func defaultSecretFileOps() secretFileOps {
	return secretFileOps{
		openExclusive: openSecretFileExclusive,
		protect:       applySecretFileProtection,
		write: func(file *os.File, body []byte) (int, error) {
			return file.Write(body)
		},
		sync:        func(file *os.File) error { return file.Sync() },
		close:       func(file *os.File) error { return file.Close() },
		remove:      os.Remove,
		replace:     replacePrivateFile,
		protectPath: applySecretPathProtection,
		syncParent:  syncParentDirectory,
	}
}

func (e *SecretFileWriteError) Error() string {
	return "authentication export write failed: " + safeSecretWriteCause(e.cause) + " (" + string(e.outcome) + ")"
}

func (e *SecretFileWriteError) Unwrap() error { return e.cause }

func (e *SecretFileWriteError) CommitOutcome() WriteCommitOutcome { return e.outcome }

func safeSecretWriteCause(err error) string {
	switch {
	case errors.Is(err, os.ErrExist):
		return "destination already exists"
	case errors.Is(err, os.ErrPermission):
		return "permission denied"
	case errors.Is(err, os.ErrNotExist):
		return "destination directory is unavailable"
	default:
		return "filesystem operation failed"
	}
}

// SecretFileWriteCommitOutcome 返回 export writer 对目标路径提交状态的判断。
func SecretFileWriteCommitOutcome(err error) WriteCommitOutcome {
	var writeErr *SecretFileWriteError
	if !errors.As(err, &writeErr) {
		return WriteCommitOutcomeUnknown
	}
	return writeErr.outcome
}

// WriteSecretFile 把 secret 写到任意目标；已有 parent 的权限与 ownership 保持不变。
// force=false 直接排他创建，因此写入期间目标可见，不承诺 atomic visibility。
func WriteSecretFile(path string, body []byte, force bool) (resultErr error) {
	return writeSecretFile(path, body, force, defaultSecretFileOps())
}

func writeSecretFile(path string, body []byte, force bool, ops secretFileOps) (resultErr error) {
	outcome := WriteCommitOutcomeNotCommitted
	defer func() {
		if resultErr != nil {
			resultErr = &SecretFileWriteError{outcome: outcome, cause: resultErr}
		}
	}()

	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	if !force {
		file, err := ops.openExclusive(path)
		if err != nil {
			return err
		}
		complete := false
		defer func() {
			if !complete {
				if err := ops.remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
					resultErr = errors.Join(resultErr, err)
				}
			}
		}()
		if err := writeAndCloseSecret(file, body, ops); err != nil {
			return err
		}
		complete = true
		outcome = WriteCommitOutcomeCommitted
		return ops.syncParent(directory)
	}

	temporary, temporaryPath, err := createSecretTemporary(directory, ops)
	if err != nil {
		return err
	}
	closed := false
	preserveTemporary := false
	defer func() {
		if !closed {
			_ = ops.close(temporary)
		}
		if !preserveTemporary {
			if err := ops.remove(temporaryPath); err != nil && !errors.Is(err, os.ErrNotExist) {
				resultErr = errors.Join(resultErr, err)
			}
		}
	}()
	if err := writeAndCloseSecret(temporary, body, ops); err != nil {
		closed = true
		return err
	}
	closed = true
	replacement, err := ops.replace(temporaryPath, path)
	preserveTemporary = replacement.preserveSource
	switch {
	case replacement.committed:
		outcome = WriteCommitOutcomeCommitted
	case replacement.preserveSource:
		outcome = WriteCommitOutcomeUnknown
	}
	if !replacement.committed {
		return err
	}
	return errors.Join(err, ops.protectPath(path), ops.syncParent(directory))
}

func writeAndCloseSecret(file *os.File, body []byte, ops secretFileOps) error {
	if err := ops.protect(file); err != nil {
		return errors.Join(err, ops.close(file))
	}
	written, err := ops.write(file, body)
	if err != nil {
		return errors.Join(err, ops.close(file))
	}
	if written != len(body) {
		return errors.Join(io.ErrShortWrite, ops.close(file))
	}
	if err := ops.sync(file); err != nil {
		return errors.Join(err, ops.close(file))
	}
	return ops.close(file)
}

func createSecretTemporary(directory string, ops secretFileOps) (*os.File, string, error) {
	for {
		var random [16]byte
		if _, err := rand.Read(random[:]); err != nil {
			return nil, "", err
		}
		path := filepath.Join(directory, ".pixiv-secret-"+hex.EncodeToString(random[:]))
		file, err := ops.openExclusive(path)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		return file, path, err
	}
}
