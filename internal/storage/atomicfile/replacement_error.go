package atomicfile

import "errors"

type replacementSourcePreservationError struct {
	err error
}

func (e replacementSourcePreservationError) Error() string { return e.err.Error() }
func (e replacementSourcePreservationError) Unwrap() error { return e.err }

// MustPreserveReplacementSource 报告 wrapped/joined error 是否要求调用方保留 source。
func MustPreserveReplacementSource(err error) bool {
	var preservationError replacementSourcePreservationError
	return errors.As(err, &preservationError)
}

func markReplacementSourceForPreservation(err error) error {
	return replacementSourcePreservationError{err: err}
}
