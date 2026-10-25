package secretfile

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/FlanChanXwO/pixiv-cli/internal/storage/atomicfile"
)

// SecretFileWriteError 隐藏任意目标路径及底层可能携带的路径，同时保留 errors.Is/As。
type SecretFileWriteError struct {
	outcome WriteCommitOutcome
	cause   error
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
		file, err := openSecretFileExclusive(path)
		if err != nil {
			return err
		}
		complete := false
		defer func() {
			if !complete {
				if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
					resultErr = errors.Join(resultErr, err)
				}
			}
		}()
		if err := writeAndCloseSecret(file, body); err != nil {
			return err
		}
		complete = true
		outcome = WriteCommitOutcomeCommitted
		return atomicfile.SyncParentDirectory(directory)
	}

	temporary, temporaryPath, err := createSecretTemporary(directory)
	if err != nil {
		return err
	}
	closed := false
	preserveTemporary := false
	defer func() {
		if !closed {
			_ = temporary.Close()
		}
		if !preserveTemporary {
			if err := os.Remove(temporaryPath); err != nil && !errors.Is(err, os.ErrNotExist) {
				resultErr = errors.Join(resultErr, err)
			}
		}
	}()
	if err := writeAndCloseSecret(temporary, body); err != nil {
		closed = true
		return err
	}
	closed = true
	replacement, err := atomicfile.ReplacePrivateFile(temporaryPath, path)
	preserveTemporary = replacement.PreserveSource
	switch {
	case replacement.Committed:
		outcome = WriteCommitOutcomeCommitted
	case replacement.PreserveSource:
		outcome = WriteCommitOutcomeUnknown
	}
	if !replacement.Committed {
		return err
	}
	return errors.Join(err, applySecretPathProtection(path), atomicfile.SyncParentDirectory(directory))
}

func writeAndCloseSecret(file *os.File, body []byte) error {
	if err := applySecretFileProtection(file); err != nil {
		return errors.Join(err, file.Close())
	}
	written, err := file.Write(body)
	if err != nil {
		return errors.Join(err, file.Close())
	}
	if written != len(body) {
		return errors.Join(io.ErrShortWrite, file.Close())
	}
	if err := file.Sync(); err != nil {
		return errors.Join(err, file.Close())
	}
	return file.Close()
}

func createSecretTemporary(directory string) (*os.File, string, error) {
	for {
		var random [16]byte
		if _, err := rand.Read(random[:]); err != nil {
			return nil, "", err
		}
		path := filepath.Join(directory, ".pixiv-secret-"+hex.EncodeToString(random[:]))
		file, err := openSecretFileExclusive(path)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		return file, path, err
	}
}
