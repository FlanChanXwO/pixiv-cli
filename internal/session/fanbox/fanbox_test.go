package fanbox_test

import (
	"context"
	"errors"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/lifecycle"
	fanboxsession "github.com/FlanChanXwO/pixiv-cli/internal/session/fanbox"
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

func newTestFacade(opener fanboxsession.AccountOpener, closeCount *int) *fanboxsession.Facade {
	return fanboxsession.NewFacadeWithCloseClient(opener, func(*fanboxsdk.Client) error {
		*closeCount++
		return nil
	})
}

func TestFacadeOpenReturnsCloseOnceLease(t *testing.T) {
	opener := &testAccountOpener{client: &fanboxsdk.Client{}}
	closeCount := 0
	facade := newTestFacade(opener, &closeCount)

	lease, err := facade.Open(context.Background(), fanboxsession.OpenRequest{})
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

	err := facade.Use(context.Background(), fanboxsession.OpenRequest{}, func(_ context.Context, client *fanboxsdk.Client, attempt *lifecycle.Attempt) error {
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
	facade := fanboxsession.NewFacade(opener)
	proxy := "http://127.0.0.1:7890"

	lease, err := facade.Open(context.Background(), fanboxsession.OpenRequest{ProxyOverride: &proxy})
	require.NoError(t, err)
	require.NoError(t, lease.Close())
	require.Equal(t, proxy, *opener.proxy)
}

func TestFacadeReportsMissingConfiguration(t *testing.T) {
	var facade *fanboxsession.Facade
	_, err := facade.Open(context.Background(), fanboxsession.OpenRequest{})
	require.ErrorIs(t, err, fanboxsession.ErrAccountServiceNotConfigured)

	err = facade.Use(context.Background(), fanboxsession.OpenRequest{}, func(context.Context, *fanboxsdk.Client, *lifecycle.Attempt) error {
		return nil
	})
	require.ErrorIs(t, err, fanboxsession.ErrAccountServiceNotConfigured)
}

func TestFacadePropagatesOpenErrorWithoutClosing(t *testing.T) {
	openErr := errors.New("open failed")
	opener := &testAccountOpener{err: openErr}
	closeCount := 0
	facade := newTestFacade(opener, &closeCount)

	_, err := facade.Open(context.Background(), fanboxsession.OpenRequest{})
	require.ErrorIs(t, err, openErr)
	require.Zero(t, closeCount)
}

func TestFacadeClosesPartialClientOnOpenError(t *testing.T) {
	partial := &fanboxsdk.Client{}
	openErr := errors.New("open failed")
	closeErr := errors.New("close failed")
	opener := &testAccountOpener{client: partial, err: openErr}
	facade := fanboxsession.NewFacadeWithCloseClient(opener, func(*fanboxsdk.Client) error {
		return closeErr
	})

	_, err := facade.Open(context.Background(), fanboxsession.OpenRequest{})
	require.ErrorIs(t, err, openErr)
	require.ErrorIs(t, err, closeErr)
}

func TestFacadeUseJoinsCallbackAndCloseErrors(t *testing.T) {
	callbackErr := errors.New("callback failed")
	closeErr := errors.New("close failed")
	opener := &testAccountOpener{client: &fanboxsdk.Client{}}
	facade := fanboxsession.NewFacadeWithCloseClient(opener, func(*fanboxsdk.Client) error {
		return closeErr
	})

	err := facade.Use(context.Background(), fanboxsession.OpenRequest{}, func(context.Context, *fanboxsdk.Client, *lifecycle.Attempt) error {
		return callbackErr
	})

	require.ErrorIs(t, err, callbackErr)
	require.ErrorIs(t, err, closeErr)
}
