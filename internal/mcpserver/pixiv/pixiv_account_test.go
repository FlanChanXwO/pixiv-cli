package pixiv_test

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/auth"
	pixivserver "github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/accounts"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	"github.com/stretchr/testify/require"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAccountToolsRegisteredWithLocalEffects(t *testing.T) {
	session, closeSession := newTestSession(t)
	defer closeSession()
	tools, err := session.ListTools(t.Context(), nil)
	require.NoError(t, err)
	for _, name := range []string{"pixiv_account_list", "pixiv_account_status", "pixiv_account_use"} {
		found := false
		for _, tool := range tools.Tools {
			if tool.Name != name {
				continue
			}
			found = true
			require.NotNil(t, tool.Annotations)
			require.False(t, tool.Annotations.ReadOnlyHint)
			require.False(t, *tool.Annotations.DestructiveHint)
			require.False(t, *tool.Annotations.OpenWorldHint)
			require.True(t, tool.Annotations.IdempotentHint)
			require.NotNil(t, tool.OutputSchema)
		}
		require.True(t, found, "missing %s", name)
	}
}

func TestAccountToolsShareSelectionWithoutOpeningSDK(t *testing.T) {
	store := auth.Store{Path: filepath.Join(t.TempDir(), "state.json")}
	_, err := store.Init(t.Context(), false)
	require.NoError(t, err)
	manager := accounts.Manager{Store: store, Load: func(context.Context) (accounts.LocalSnapshot, error) {
		return accounts.LocalSnapshot{Accounts: []accounts.Account{{UserID: 42, Username: "first", HasCredentials: true}, {UserID: 73, Username: "second", HasCredentials: true}}}, nil
	}}
	ports := pixivserver.SDKPorts{Accounts: manager}
	first, closeFirst := newSDKTestSessionWithPorts(t, ports, pixivserver.Account{})
	defer closeFirst()
	second, closeSecond := newSDKTestSessionWithPorts(t, ports, pixivserver.Account{})
	defer closeSecond()
	result := callTool(t, first, "pixiv_account_list", map[string]any{})
	require.False(t, result.IsError)
	var out outputs.AccountStatus
	decodeStructured(t, result, &out)
	require.Equal(t, "selection_required", out.SelectionState)
	require.Len(t, out.Accounts, 2)
	result = callTool(t, first, "pixiv_account_use", map[string]any{"user_id": 73})
	require.False(t, result.IsError)
	decodeStructured(t, result, &out)
	require.Equal(t, int64(73), out.SelectedUserID)
	result = callTool(t, second, "pixiv_account_status", map[string]any{})
	require.False(t, result.IsError)
	decodeStructured(t, result, &out)
	require.Equal(t, int64(73), out.SelectedUserID)
	require.Equal(t, "present_unverified", out.CredentialState)
	for _, args := range []map[string]any{{}, {"user_id": 0}, {"user_id": -1}, {"user_id": 999}, {"user_id": 73, "refresh_token": "not-accepted"}} {
		result = callTool(t, first, "pixiv_account_use", args)
		require.True(t, result.IsError)
	}
	saved, err := store.Read(t.Context())
	require.NoError(t, err)
	require.Equal(t, int64(73), saved.SelectedPixivUserID)
}

func TestAccountToolsRedactLocalErrorsAndPreserveStructuredFailure(t *testing.T) {
	manager := accounts.Manager{Load: func(context.Context) (accounts.LocalSnapshot, error) {
		return accounts.LocalSnapshot{}, errors.New("fixture-secret /private/database-path")
	}}
	session, closeSession := newSDKTestSessionWithPorts(t, pixivserver.SDKPorts{Accounts: manager}, pixivserver.Account{})
	defer closeSession()
	for _, name := range []string{"pixiv_account_list", "pixiv_account_status", "pixiv_account_use"} {
		args := map[string]any{}
		if name == "pixiv_account_use" {
			args["user_id"] = 73
		}
		result := callTool(t, session, name, args)
		require.True(t, result.IsError)
		body, err := json.Marshal(result)
		require.NoError(t, err)
		require.NotContains(t, string(body), "fixture-secret")
		require.NotContains(t, string(body), "database-path")
		var out outputs.AccountStatus
		decodeStructured(t, result, &out)
		require.Equal(t, "local_state_error", out.Error)
		require.Equal(t, "unknown", out.SelectionState)
		require.Empty(t, out.Accounts)
	}
}

func TestAccountUseDoesNotReportSuccessWhenStateIsUnavailable(t *testing.T) {
	file := filepath.Join(t.TempDir(), "not-a-directory")
	require.NoError(t, os.WriteFile(file, []byte("unchanged"), 0600))
	manager := accounts.Manager{Store: auth.Store{Path: filepath.Join(file, "state.json")}, Load: func(context.Context) (accounts.LocalSnapshot, error) {
		return accounts.LocalSnapshot{Accounts: []accounts.Account{{UserID: 73, HasCredentials: true}}}, nil
	}}
	session, closeSession := newSDKTestSessionWithPorts(t, pixivserver.SDKPorts{Accounts: manager}, pixivserver.Account{})
	defer closeSession()
	result := callTool(t, session, "pixiv_account_use", map[string]any{"user_id": 73})
	require.True(t, result.IsError)
	var out outputs.AccountStatus
	decodeStructured(t, result, &out)
	require.Equal(t, "local_state_error", out.Error)
	body, err := os.ReadFile(file)
	require.NoError(t, err)
	require.Equal(t, "unchanged", string(body))
}

func TestAccountSchemaErrorsDoNotEchoRawInput(t *testing.T) {
	session, closeSession := newTestSession(t)
	defer closeSession()
	result := callTool(t, session, "pixiv_account_use", map[string]any{"user_id": "fixture-secret-value"})
	require.True(t, result.IsError)
	body, err := json.Marshal(result)
	require.NoError(t, err)
	require.NotContains(t, string(body), "fixture-secret-value")
	require.NotNil(t, result.StructuredContent)
}

func TestLoginToolsSharePendingSessionAndQueryWithoutLocalState(t *testing.T) {
	manager, err := accounts.NewLoginManager(t.Context(), "https://relay.example/pixiv-login", func() (accounts.LoginAttempt, error) {
		return accounts.LoginAttempt{AuthorizationURL: "https://app-api.pixiv.net/web/v1/login", AcceptsCallback: func(string) bool { return true }, Complete: func(context.Context, string) (accounts.LoginResult, error) { return accounts.LoginResult{}, nil }}, nil
	})
	require.NoError(t, err)
	defer manager.Close()
	ports := pixivserver.SDKPorts{Login: manager}
	first, closeFirst := newSDKTestSessionWithPorts(t, ports, pixivserver.Account{})
	defer closeFirst()
	second, closeSecond := newSDKTestSessionWithPorts(t, ports, pixivserver.Account{})
	defer closeSecond()
	tools, err := first.ListTools(t.Context(), nil)
	require.NoError(t, err)
	found := false
	for _, tool := range tools.Tools {
		if tool.Name == "pixiv_account_login_start" {
			found = true
			require.False(t, tool.Annotations.ReadOnlyHint)
			require.True(t, *tool.Annotations.DestructiveHint)
			require.False(t, tool.Annotations.IdempotentHint)
			require.True(t, *tool.Annotations.OpenWorldHint)
		}
	}
	require.True(t, found, "missing real login-start tool")
	result := callTool(t, first, "pixiv_account_login_start", map[string]any{})
	require.False(t, result.IsError)
	var started struct {
		LoginID      string `json:"login_id"`
		URL          string `json:"authorization_url"`
		Status       string `json:"status"`
		Helper       bool   `json:"requires_local_helper"`
		Instructions string `json:"instructions"`
	}
	decodeStructured(t, result, &started)
	require.NotEmpty(t, started.LoginID)
	require.Contains(t, started.URL, "https://relay.example/pixiv-login/session/")
	require.Equal(t, "waiting_for_user", started.Status)
	require.True(t, started.Helper)
	require.NotEmpty(t, started.Instructions)
	again := callTool(t, second, "pixiv_account_login_start", map[string]any{})
	require.False(t, again.IsError)
	var reused struct {
		LoginID string `json:"login_id"`
	}
	decodeStructured(t, again, &reused)
	require.Equal(t, started.LoginID, reused.LoginID)
	status := callTool(t, second, "pixiv_account_status", map[string]any{"login_id": started.LoginID})
	require.False(t, status.IsError)
	var queried struct {
		Login accounts.LoginStatus `json:"login"`
	}
	decodeStructured(t, status, &queried)
	require.Equal(t, "waiting_for_user", queried.Login.Status)
	require.Equal(t, started.LoginID, queried.Login.LoginID)
	restart := callTool(t, second, "pixiv_account_login_start", map[string]any{"restart": true})
	require.False(t, restart.IsError)
	decodeStructured(t, restart, &reused)
	require.NotEqual(t, started.LoginID, reused.LoginID)
	status = callTool(t, first, "pixiv_account_status", map[string]any{"login_id": started.LoginID})
	decodeStructured(t, status, &queried)
	require.Equal(t, "not_found", queried.Login.Status)
}

func TestLoginStartErrorsKeepLoginEnvelopeAndHideArguments(t *testing.T) {
	session, closeSession := newSDKTestSessionWithPorts(t, pixivserver.SDKPorts{}, pixivserver.Account{})
	defer closeSession()
	for _, args := range []map[string]any{{"restart": "fixture-secret"}, {"callback_url": "fixture-secret"}} {
		result := callTool(t, session, "pixiv_account_login_start", args)
		require.True(t, result.IsError)
		var out outputs.LoginStatus
		decodeStructured(t, result, &out)
		require.Equal(t, "failed", out.Status)
		require.Equal(t, "invalid_request", out.Error)
		body, err := json.Marshal(result)
		require.NoError(t, err)
		require.NotContains(t, string(body), "fixture-secret")
	}
	result := callTool(t, session, "pixiv_account_login_start", map[string]any{})
	require.True(t, result.IsError)
	var out outputs.LoginStatus
	decodeStructured(t, result, &out)
	require.Equal(t, "login_start_failed", out.Error)
}

func TestLoginStatusToolRetainsSavedAccountOnSelectionFailure(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	manager, err := accounts.NewLoginManager(t.Context(), "https://relay.example", func() (accounts.LoginAttempt, error) {
		return accounts.LoginAttempt{
			AuthorizationURL: "https://app-api.pixiv.net/web/v1/login", AcceptsCallback: func(string) bool { return true },
			Complete: func(ctx context.Context, _ string) (accounts.LoginResult, error) {
				close(entered)
				select {
				case <-release:
				case <-ctx.Done():
					return accounts.LoginResult{}, ctx.Err()
				}
				return accounts.LoginResult{Account: accounts.Account{UserID: 73, HasCredentials: true}, AccountSaved: true}, errors.New("fixture-private-store-path")
			},
		}, nil
	})
	require.NoError(t, err)
	defer manager.Close()
	session, closeSession := newSDKTestSessionWithPorts(t, pixivserver.SDKPorts{Login: manager}, pixivserver.Account{})
	defer closeSession()
	result := callTool(t, session, "pixiv_account_login_start", map[string]any{})
	var started outputs.LoginStatus
	decodeStructured(t, result, &started)
	page := httptest.NewRecorder()
	manager.ServeHTTP(page, httptest.NewRequest("GET", started.AuthorizationURL, nil))
	link, err := url.Parse(page.Header().Get("Location"))
	require.NoError(t, err)
	id, proof := link.Query().Get("session"), link.Query().Get("access")
	start := httptest.NewRecorder()
	manager.ServeHTTP(start, httptest.NewRequest("POST", "/start/"+id, strings.NewReader(`{"proof":"`+proof+`"}`)))
	require.Equal(t, 200, start.Code)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan struct{})
	go func() {
		manager.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/callback/"+id, strings.NewReader(`{"proof":"`+proof+`","callback_url":"pixiv://account/login?code=fixture"}`)).WithContext(ctx))
		close(done)
	}()
	<-entered
	result = callTool(t, session, "pixiv_account_status", map[string]any{"login_id": started.LoginID})
	var status outputs.AccountStatus
	decodeStructured(t, result, &status)
	require.Equal(t, "exchanging", status.Login.Status)
	cancel()
	<-done
	close(release)
	require.Eventually(t, func() bool { return manager.Status(started.LoginID).Status == "failed" }, 5*time.Second, time.Millisecond)
	result = callTool(t, session, "pixiv_account_status", map[string]any{"login_id": started.LoginID})
	require.True(t, result.IsError)
	decodeStructured(t, result, &status)
	require.Equal(t, "failed", status.Login.Status)
	require.True(t, status.Login.AccountSaved)
	require.False(t, status.Login.SelectionUpdated)
	require.Equal(t, int64(73), status.Login.Account.UserID)
	body, err := json.Marshal(result)
	require.NoError(t, err)
	require.NotContains(t, string(body), "fixture-private-store-path")
	require.NotContains(t, string(body), proof)
}
