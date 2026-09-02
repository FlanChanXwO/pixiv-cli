package reversesearch_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	reversesearch "github.com/FlanChanXwO/pixiv-cli/internal/services/reversesearch"
	"github.com/stretchr/testify/require"
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

func TestFacadeCloseClosesPayloadSearcherOnceAndPreservesError(t *testing.T) {
	closeErr := errors.New("payload close failed")
	payloads := &closeablePayloadSearcher{closeErr: closeErr}
	facade := reversesearch.NewFacade(reversesearch.Dependencies{Payloads: payloads})

	closer, ok := any(facade).(reversesearch.Closer)
	require.True(t, ok, "facade should expose the internal lifecycle contract")
	require.ErrorIs(t, closer.Close(), closeErr)
	require.ErrorIs(t, closer.Close(), closeErr)
	require.Equal(t, int32(1), payloads.closeCalls.Load())
}

type closeableProviderClient struct {
	closeCalls atomic.Int32
}

func (s *closeableProviderClient) Preflight(context.Context) error { return nil }

func (s *closeableProviderClient) Search(context.Context, *reversesearch.Snapshot) (reversesearch.ProviderResponse, error) {
	return reversesearch.ProviderResponse{}, nil
}

func (s *closeableProviderClient) Close() error {
	s.closeCalls.Add(1)
	return nil
}

type closeableASCII2DClient struct {
	closeCalls atomic.Int32
}

func (s *closeableASCII2DClient) Preflight(context.Context) error { return nil }

func (s *closeableASCII2DClient) Upload(context.Context, *reversesearch.Snapshot) (reversesearch.ASCII2DSession, error) {
	return nil, nil
}

func (s *closeableASCII2DClient) Close() error {
	s.closeCalls.Add(1)
	return nil
}

func TestAggregatorCloseClosesProviderClientsOnce(t *testing.T) {
	sauce := &closeableProviderClient{}
	ascii := &closeableASCII2DClient{}
	aggregator := reversesearch.NewAggregator(reversesearch.AggregatorDependencies{
		SauceNAO: sauce,
		ASCII2D:  ascii,
	})

	closer, ok := any(aggregator).(reversesearch.Closer)
	require.True(t, ok, "aggregator should expose the internal lifecycle contract")
	require.NoError(t, closer.Close())
	require.NoError(t, closer.Close())
	require.Equal(t, int32(1), sauce.closeCalls.Load())
	require.Equal(t, int32(1), ascii.closeCalls.Load())
}
