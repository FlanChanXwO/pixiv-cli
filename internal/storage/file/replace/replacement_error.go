package replace

import "errors"

// ReplacementSourcePreservationError 标记一次替换错误后的 source 仍是恢复材料。
// 调用方必须保留 source，直至人工或后续恢复流程完成。
type ReplacementSourcePreservationError interface {
	error
	PreserveReplacementSource()
}

type replacementSourcePreservationError struct {
	err error
}

func (e replacementSourcePreservationError) Error() string            { return e.err.Error() }
func (e replacementSourcePreservationError) Unwrap() error            { return e.err }
func (replacementSourcePreservationError) PreserveReplacementSource() {}

// MustPreserveReplacementSource 报告 wrapped/joined error 是否要求调用方保留 source。
func MustPreserveReplacementSource(err error) bool {
	var preservationError ReplacementSourcePreservationError
	return errors.As(err, &preservationError)
}

func markReplacementSourceForPreservation(err error) error {
	if MustPreserveReplacementSource(err) {
		return err
	}
	return replacementSourcePreservationError{err: err}
}
