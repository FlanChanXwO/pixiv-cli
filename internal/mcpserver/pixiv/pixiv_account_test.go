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
	"os"
	"path/filepath"
	"testing"
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
