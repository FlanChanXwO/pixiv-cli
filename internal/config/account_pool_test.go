package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRuntimeAccountPoolRequiresValidExplicitWhitelistWhenEnabled(t *testing.T) {
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
			want: AccountPoolConfig{Enabled: true, Accounts: []int64{11, 22}, Strategy: AccountPoolStrategyRoundRobin},
		},
		{
			name: "random",
			body: "[account_pool]\nenabled = true\naccounts = [11, 22]\nstrategy = 'random'\n",
			want: AccountPoolConfig{Enabled: true, Accounts: []int64{11, 22}, Strategy: AccountPoolStrategyRandom},
		},
		{name: "enabled without accounts", body: "[account_pool]\nenabled = true\n", wantErr: "account_pool.accounts must contain at least one UID"},
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

func TestAccountPoolIsNotExposedAsAConfigSetAlias(t *testing.T) {
	for _, alias := range []string{"account_pool", "account_pool_enabled", "account_pool_accounts", "account_pool_strategy"} {
		_, exists := SettingSpecByAlias(alias)
		require.Falsef(t, exists, "%s must remain a hand-maintained config.toml table", alias)
	}
}
