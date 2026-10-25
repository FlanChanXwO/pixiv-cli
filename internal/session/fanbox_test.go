package session

import (
	"context"
	"errors"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/lifecycle"
	fanboxsdk "github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
	"github.com/stretchr/testify/require"
)

type testAccountOpener struct {
	client    *fanboxsdk.Client
	err       error
	proxy     *string
	openCount int
}

func (o *testAccountOpener) OpenClient(_ context.Context, proxy *string) (*fanboxsdk.Client, error) {
	o.openCount++
	o.proxy = proxy
	return o.client, o.err
}

func newTestFanbox(opener AccountOpener, closeCount *int) *Fanbox {
	f := NewFanbox(opener)
	f.closeClient = func(*fanboxsdk.Client) error {
		*closeCount++
		return nil
	}
	return f
}

func TestFanboxPassesProxyOverrideToAccountLeaf(t *testing.T) {
	opener := &testAccountOpener{client: &fanboxsdk.Client{}}
	facade := NewFanbox(opener)
	proxy := "http://127.0.0.1:7890"

	lease, err := facade.Open(context.Background(), OpenRequest{ProxyOverride: &proxy})
	require.NoError(t, err)
	require.NoError(t, lease.Close())
	require.Equal(t, proxy, *opener.proxy)
}

func TestFanboxReportsMissingConfiguration(t *testing.T) {
	var facade *Fanbox
	_, err := facade.Open(context.Background(), OpenRequest{})
	require.ErrorIs(t, err, fanboxAccountServiceNotConfigured)

	err = facade.Use(context.Background(), OpenRequest{}, func(context.Context, *fanboxsdk.Client, *lifecycle.Attempt) error {
		return nil
	})
	require.ErrorIs(t, err, fanboxAccountServiceNotConfigured)
}

func TestFanboxPropagatesOpenErrorWithoutClosing(t *testing.T) {
	openErr := errors.New("open failed")
	opener := &testAccountOpener{err: openErr}
	closeCount := 0
	facade := newTestFanbox(opener, &closeCount)

	_, err := facade.Open(context.Background(), OpenRequest{})
	require.ErrorIs(t, err, openErr)
	require.Zero(t, closeCount)
}

func TestFanboxClosesPartialClientOnOpenError(t *testing.T) {
	partial := &fanboxsdk.Client{}
	openErr := errors.New("open failed")
	closeErr := errors.New("close failed")
	opener := &testAccountOpener{client: partial, err: openErr}
	facade := NewFanbox(opener)
	facade.closeClient = func(*fanboxsdk.Client) error { return closeErr }

	_, err := facade.Open(context.Background(), OpenRequest{})
	require.ErrorIs(t, err, openErr)
	require.ErrorIs(t, err, closeErr)
}

func TestFanboxUseJoinsCallbackAndCloseErrors(t *testing.T) {
	callbackErr := errors.New("callback failed")
	closeErr := errors.New("close failed")
	opener := &testAccountOpener{client: &fanboxsdk.Client{}}
	facade := NewFanbox(opener)
	facade.closeClient = func(*fanboxsdk.Client) error { return closeErr }

	err := facade.Use(context.Background(), OpenRequest{}, func(context.Context, *fanboxsdk.Client, *lifecycle.Attempt) error {
		return callbackErr
	})

	require.ErrorIs(t, err, callbackErr)
	require.ErrorIs(t, err, closeErr)
}
