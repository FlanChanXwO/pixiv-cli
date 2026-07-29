package application

import (
	"context"
	"errors"
	"net/url"
	"testing"

	sdk "github.com/FlanChanXwO/pixiv-cli/pixiv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoginServiceStartPreservesPublicSessionBehavior(t *testing.T) {
	publicClient, err := sdk.NewClient(sdk.NewClientOptions{})
	require.NoError(t, err)
	session, err := publicClient.StartLogin()
	require.NoError(t, err)
	fake := &fakeLoginSDKClient{startSession: session}
	proxy := "http://proxy.invalid:7890"
	wantRequest := SDKClientRequest{UserID: 123, HTTPSProxyOverride: &proxy}
	var gotRequest SDKClientRequest
	service := LoginService{SDK: SDKService{NewClient: func(request SDKClientRequest) (SDKClient, error) {
		gotRequest = request
		return fake, nil
	}}}

	start, err := service.Start(wantRequest)
	require.NoError(t, err)
	assert.Equal(t, wantRequest, gotRequest)
	assert.Equal(t, session.AuthorizationURL(), start.AuthorizationURL)

	parsed, err := url.Parse(start.AuthorizationURL)
	require.NoError(t, err)
	state := parsed.Query().Get("state")
	require.NotEmpty(t, state)
	foreign := sdk.OAuthCallbackURLPrefix() + "code=foreign&state=other"
	matching := sdk.OAuthCallbackURLPrefix() + "code=accepted&state=" + url.QueryEscape(state)
	assert.False(t, start.AcceptsCallbackURL(foreign))
	assert.True(t, start.AcceptsCallbackURL(matching))
}

func TestLoginServiceStartPropagatesDependencyErrors(t *testing.T) {
	factoryErr := errors.New("open sdk dependency")
	_, err := (LoginService{SDK: SDKService{NewClient: func(SDKClientRequest) (SDKClient, error) {
		return nil, factoryErr
	}}}).Start()
	require.ErrorIs(t, err, factoryErr)

	startErr := errors.New("start login dependency")
	_, err = (LoginService{SDK: SDKService{NewClient: func(SDKClientRequest) (SDKClient, error) {
		return &fakeLoginSDKClient{startErr: startErr}, nil
	}}}).Start()
	require.ErrorIs(t, err, startErr)
}

func TestLoginServiceCompleteHandsOffSessionAndOptions(t *testing.T) {
	publicClient, err := sdk.NewClient(sdk.NewClientOptions{})
	require.NoError(t, err)
	session, err := publicClient.StartLogin()
	require.NoError(t, err)
	fake := &fakeLoginSDKClient{
		startSession: session,
		completeAccount: &sdk.Account{
			UserID:   456,
			Username: "alice",
			Default:  true,
			HasToken: true,
		},
	}
	service := LoginService{SDK: SDKService{NewClient: func(SDKClientRequest) (SDKClient, error) {
		return fake, nil
	}}}
	start, err := service.Start()
	require.NoError(t, err)

	result, err := service.Complete(context.Background(), start, LoginCompleteRequest{
		CallbackOrCode: "authorization-code",
		UseAfterLogin:  true,
	})
	require.NoError(t, err)
	assert.Equal(t, AccountResult{UserID: 456, Username: "alice", Default: true, HasToken: true}, result)
	assert.Same(t, session, fake.completeSession)
	assert.Equal(t, "authorization-code", fake.completeInput)
	assert.Equal(t, sdk.LoginOptions{UseAsDefault: true}, fake.completeOptions)
}

func TestLoginServiceCompletePropagatesErrorsAndRejectsMissingSession(t *testing.T) {
	_, err := (LoginService{}).Complete(context.Background(), LoginStart{}, LoginCompleteRequest{})
	require.EqualError(t, err, "login session is not initialized")

	publicClient, err := sdk.NewClient(sdk.NewClientOptions{})
	require.NoError(t, err)
	session, err := publicClient.StartLogin()
	require.NoError(t, err)
	completeErr := errors.New("complete login dependency")
	fake := &fakeLoginSDKClient{startSession: session, completeErr: completeErr}
	service := LoginService{SDK: SDKService{NewClient: func(SDKClientRequest) (SDKClient, error) {
		return fake, nil
	}}}
	start, err := service.Start()
	require.NoError(t, err)

	_, err = service.Complete(context.Background(), start, LoginCompleteRequest{CallbackOrCode: "code"})
	require.ErrorIs(t, err, completeErr)
}

// fakeLoginSDKClient 只实现登录用例触达的 public SDK facade 方法。
type fakeLoginSDKClient struct {
	SDKClient
	startSession    *sdk.LoginSession
	startErr        error
	completeAccount *sdk.Account
	completeErr     error
	completeSession *sdk.LoginSession
	completeInput   string
	completeOptions sdk.LoginOptions
}

func (f *fakeLoginSDKClient) StartLogin() (*sdk.LoginSession, error) {
	return f.startSession, f.startErr
}

func (f *fakeLoginSDKClient) CompleteLogin(_ context.Context, session *sdk.LoginSession, input string, options sdk.LoginOptions) (*sdk.Account, error) {
	f.completeSession = session
	f.completeInput = input
	f.completeOptions = options
	return f.completeAccount, f.completeErr
}
