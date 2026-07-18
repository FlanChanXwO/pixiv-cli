package files

import "errors"

// WriteCommitOutcome 标识 private-file replacement 是否已经提交到目标路径。
type WriteCommitOutcome string

const (
	WriteCommitOutcomeUnknown      WriteCommitOutcome = "unknown"
	WriteCommitOutcomeNotCommitted WriteCommitOutcome = "not_committed"
	WriteCommitOutcomeCommitted    WriteCommitOutcome = "committed"
)

type privateFileWriteError struct {
	outcome WriteCommitOutcome
	cause   error
}

func (e *privateFileWriteError) Error() string { return e.cause.Error() }
func (e *privateFileWriteError) Unwrap() error { return e.cause }

// PrivateFileWriteCommitOutcome 返回 private writer 错误的稳定 commit outcome。
// 非本 writer 产生的错误返回 unknown。
func PrivateFileWriteCommitOutcome(err error) WriteCommitOutcome {
	var writeErr *privateFileWriteError
	if !errors.As(err, &writeErr) {
		return WriteCommitOutcomeUnknown
	}
	return writeErr.outcome
}

func withPrivateFileWriteCommitOutcome(err error, committed bool) error {
	if err == nil {
		return nil
	}
	outcome := WriteCommitOutcomeNotCommitted
	if committed {
		outcome = WriteCommitOutcomeCommitted
	}
	return &privateFileWriteError{outcome: outcome, cause: err}
}
