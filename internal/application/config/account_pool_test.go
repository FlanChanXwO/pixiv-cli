package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRuntimeAccountPoolIgnoresLegacyUIDAllowlistAfterValidation(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr string
		want    AccountPoolConfig
	}{
		{
			name: "disabled by default",
			want: AccountPoolConfig{Strategy: AccountPoolStrategyRoundRobin},
		},
		{
			name: "enabled round robin",
			body: "[account_pool]\nenabled = true\naccounts = [11, 22]\nstrategy = 'round_robin'\n",
			want: AccountPoolConfig{Enabled: true, Strategy: AccountPoolStrategyRoundRobin},
		},
		{
			name: "random",
			body: "[account_pool]\nenabled = true\naccounts = [11, 22]\nstrategy = 'random'\n",
			want: AccountPoolConfig{Enabled: true, Strategy: AccountPoolStrategyRandom},
		},
		{name: "enabled without accounts uses all local accounts", body: "[account_pool]\nenabled = true\n", want: AccountPoolConfig{Enabled: true, Strategy: AccountPoolStrategyRoundRobin}},
		{name: "duplicate UID", body: "[account_pool]\nenabled = true\naccounts = [11, 11]\n", wantErr: "account_pool.accounts must not contain duplicate UIDs"},
		{name: "non positive UID", body: "[account_pool]\nenabled = true\naccounts = [0]\n", wantErr: "account_pool.accounts must contain only positive UIDs"},
		{name: "unknown strategy", body: "[account_pool]\nstrategy = 'weighted'\n", wantErr: "account_pool.strategy must be one of: round_robin, random"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.toml")
			if test.body != "" {
				require.NoError(t, os.WriteFile(path, []byte(test.body), 0o600))
			}
			state, err := LoadSettingsStateAt(path)
			require.NoError(t, err)
			runtime, err := state.Runtime()
			if test.wantErr != "" {
				require.EqualError(t, err, test.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, test.want, runtime.AccountPool)
		})
	}
}

func TestAccountPoolAliasesExposeOnlyRuntimeSettings(t *testing.T) {
	for _, alias := range []string{"account_pool_enabled", "account_pool_strategy"} {
		_, exists := SettingSpecByAlias(alias)
		require.Truef(t, exists, "%s must be a supported config alias", alias)
	}
	_, exists := SettingSpecByAlias("account_pool_accounts")
	require.False(t, exists, "UID allowlist must not remain a runtime config alias")
}

func TestLegacyAccountPoolUIDsAreParsedWithoutEnteringRuntimeConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, os.WriteFile(path, []byte("[account_pool]\nenabled=true\naccounts=[11,22]\nstrategy='random'\n"), 0o600))
	state, err := LoadSettingsStateAt(path)
	require.NoError(t, err)
	ids, present, err := state.LegacyAccountPoolUIDs()
	require.NoError(t, err)
	require.True(t, present)
	require.Equal(t, []int64{11, 22}, ids)

	require.NoError(t, RemoveLegacyAccountPoolAccounts(path))
	body, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NotContains(t, string(body), "accounts")
	require.Contains(t, string(body), "enabled")
	require.Contains(t, string(body), "strategy")
}
