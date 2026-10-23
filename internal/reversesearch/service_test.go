package reversesearch_test

import (
	"context"
	"errors"
	"github.com/FlanChanXwO/pixiv-cli/internal/reversesearch"
	"github.com/stretchr/testify/require"
	"sync/atomic"
	"testing"
)

type closeablePayloadSearcher struct {
	closeCalls atomic.Int32
	closeErr   error
}

func (s *closeablePayloadSearcher) Preflight(context.Context, reversesearch.PayloadQuery) error {
	return nil
}

func (s *closeablePayloadSearcher) SearchPayload(context.Context, reversesearch.PayloadRequest) (reversesearch.Response, error) {
	return reversesearch.Response{}, nil
}

func (s *closeablePayloadSearcher) Close() error {
	s.closeCalls.Add(1)
	return s.closeErr
}

func TestServiceCloseClosesPayloadSearcherOnceAndPreservesError(t *testing.T) {
	closeErr := errors.New("payload close failed")
	payloads := &closeablePayloadSearcher{closeErr: closeErr}
	service := reversesearch.NewService(nil, payloads)

	closer, ok := any(service).(reversesearch.Closer)
	require.True(t, ok, "service should expose the internal lifecycle contract")
	require.ErrorIs(t, closer.Close(), closeErr)
	require.ErrorIs(t, closer.Close(), closeErr)
	require.Equal(t, int32(1), payloads.closeCalls.Load())
}
