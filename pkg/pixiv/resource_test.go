package pixiv

import (
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClassifyResourceCopyErrorDistinguishesDestinationFailure(t *testing.T) {
	for _, writer := range []io.Writer{
		resourceFailingWriter{err: errors.New("write canary")},
		resourceShortWriter{},
	} {
		tracked := &resourceDestinationWriter{writer: writer}
		_, copyErr := io.Copy(tracked, oneReadReader{})
		require.Error(t, copyErr)
		err := classifyResourceCopyError(copyErr, tracked.err)
		require.ErrorIs(t, err, ErrInvalidArgument)
		var typed *Error
		require.True(t, errors.As(err, &typed))
		assert.Equal(t, OperationDownload, typed.Operation)
		assert.Empty(t, typed.Backend)
		assert.False(t, typed.Retryable)
		assert.NotContains(t, err.Error(), "canary")
	}
}

type resourceFailingWriter struct{ err error }

func (w resourceFailingWriter) Write([]byte) (int, error) { return 0, w.err }

type resourceShortWriter struct{}

func (resourceShortWriter) Write(payload []byte) (int, error) { return len(payload) - 1, nil }

type oneReadReader struct{ read bool }

func (r oneReadReader) Read(payload []byte) (int, error) {
	payload[0] = 'x'
	return 1, io.EOF
}
