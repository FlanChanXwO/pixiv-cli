package settings_test

import (
	"testing"

	config "github.com/FlanChanXwO/pixiv-cli/internal/config/settings"
	"github.com/stretchr/testify/require"
)

// TestDeclaredKeysResolve guards the Key constants against typos: every
// constant must name a declared alias, and the declared aliases must be fully
// covered by constants.
func TestDeclaredKeysResolve(t *testing.T) {
	keys := []config.Key{
		config.KeyAccountPoolEnabled,
		config.KeyAccountPoolStrategy,
		config.KeyDirectoryTemplate,
		config.KeyDownloadPath,
		config.KeyFilenameTemplate,
		config.KeyHTTPSProxy,
		config.KeyLogFormat,
		config.KeyLogLevel,
		config.KeyLoginOpenBrowser,
		config.KeyLoginRelayListenAddr,
		config.KeyLoginRelayPublicURL,
		config.KeyLoginRelayTLSCertFile,
		config.KeyLoginRelayTLSKeyFile,
		config.KeyLoginUseAfterLogin,
		config.KeyMCPGalleryAnimationTempCapacityMiB,
		config.KeyMCPGalleryCacheCapacityMiB,
		config.KeyOutputJSON,
		config.KeyRequestInterval,
		config.KeyReverseSearchPixivOnly,
		config.KeyReverseSearchProvider,
		config.KeySauceNAOAPIKey,
		config.KeyUpdateCheckEnabled,
	}

	declared := make(map[config.Key]bool)
	for _, key := range keys {
		t.Run(string(key), func(t *testing.T) {
			spec, ok := config.SettingSpecByAlias(key)
			require.Truef(t, ok, "Key constant %q has no declared alias", key)
			require.Equal(t, key, spec.Alias)
			declared[key] = true
		})
	}

	for _, key := range config.ValidSettingAliases() {
		require.Truef(t, declared[key], "declared alias %q has no Key constant", key)
	}
}
