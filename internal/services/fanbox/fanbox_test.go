package fanbox

import (
	"context"
	"errors"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/shared/lifecycle"
	fanboxsdk "github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
	"github.com/stretchr/testify/require"
)

type testAccountOpener struct {
	client    *fanboxsdk.Client
	err       error
	proxy     *string
	openCount int
}

func (o *testAccountOpener) OpenClientWithProxy(_ context.Context, proxy *string) (*fanboxsdk.Client, error) {
	o.openCount++
	o.proxy = proxy
	return o.client, o.err
}

func newTestFacade(opener AccountOpener, closeCount *int) *Facade {
	return NewFacadeWithCloseClient(opener, func(*fanboxsdk.Client) error {
		*closeCount++
		return nil
	})
}

func TestFacadeOpenReturnsCloseOnceLease(t *testing.T) {
	opener := &testAccountOpener{client: &fanboxsdk.Client{}}
	closeCount := 0
	facade := newTestFacade(opener, &closeCount)

	lease, err := facade.Open(context.Background(), OpenRequest{})
	require.NoError(t, err)
	require.Same(t, opener.client, lease.Value())
	require.NoError(t, lease.Close())
	require.NoError(t, lease.Close())
	require.Equal(t, 1, closeCount)
	require.Equal(t, 1, opener.openCount)
}

func TestFacadeUseClosesClientAfterCallback(t *testing.T) {
	opener := &testAccountOpener{client: &fanboxsdk.Client{}}
	closeCount := 0
	facade := newTestFacade(opener, &closeCount)
	callbackErr := errors.New("callback failed")
	var callbackClient *fanboxsdk.Client
	var callbackAttempt *lifecycle.Attempt

	err := facade.Use(context.Background(), OpenRequest{}, func(_ context.Context, client *fanboxsdk.Client, attempt *lifecycle.Attempt) error {
		callbackClient = client
		callbackAttempt = attempt
		return callbackErr
	})

	require.ErrorIs(t, err, callbackErr)
	require.Same(t, opener.client, callbackClient)
	require.NotNil(t, callbackAttempt)
	require.Equal(t, 1, closeCount)
}

func TestFacadePassesProxyOverrideToAccountLeaf(t *testing.T) {
	opener := &testAccountOpener{client: &fanboxsdk.Client{}}
	facade := NewFacade(opener)
	proxy := "http://127.0.0.1:7890"

	lease, err := facade.Open(context.Background(), OpenRequest{ProxyOverride: &proxy})
	require.NoError(t, err)
	require.NoError(t, lease.Close())
	require.Equal(t, proxy, *opener.proxy)
}

func TestFacadeReportsMissingConfiguration(t *testing.T) {
	var facade *Facade
	_, err := facade.Open(context.Background(), OpenRequest{})
	require.ErrorIs(t, err, ErrAccountServiceNotConfigured)

	err = facade.Use(context.Background(), OpenRequest{}, func(context.Context, *fanboxsdk.Client, *lifecycle.Attempt) error {
		return nil
	})
	require.ErrorIs(t, err, ErrAccountServiceNotConfigured)
}

func TestFacadePropagatesOpenErrorWithoutClosing(t *testing.T) {
	openErr := errors.New("open failed")
	opener := &testAccountOpener{err: openErr}
	closeCount := 0
	facade := newTestFacade(opener, &closeCount)

	_, err := facade.Open(context.Background(), OpenRequest{})
	require.ErrorIs(t, err, openErr)
	require.Zero(t, closeCount)
}

func TestFacadeClosesPartialClientOnOpenError(t *testing.T) {
	partial := &fanboxsdk.Client{}
	openErr := errors.New("open failed")
	closeErr := errors.New("close failed")
	opener := &testAccountOpener{client: partial, err: openErr}
	facade := NewFacadeWithCloseClient(opener, func(*fanboxsdk.Client) error {
		return closeErr
	})

	_, err := facade.Open(context.Background(), OpenRequest{})
	require.ErrorIs(t, err, openErr)
	require.ErrorIs(t, err, closeErr)
}

func TestFacadeUseJoinsCallbackAndCloseErrors(t *testing.T) {
	callbackErr := errors.New("callback failed")
	closeErr := errors.New("close failed")
	opener := &testAccountOpener{client: &fanboxsdk.Client{}}
	facade := NewFacadeWithCloseClient(opener, func(*fanboxsdk.Client) error {
		return closeErr
	})

	err := facade.Use(context.Background(), OpenRequest{}, func(context.Context, *fanboxsdk.Client, *lifecycle.Attempt) error {
		return callbackErr
	})

	require.ErrorIs(t, err, callbackErr)
	require.ErrorIs(t, err, closeErr)
}
