package config

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/i18n"
)

func fields(c *Config) map[string]any {
	return map[string]any{
		VectorModelKey: &c.Vector.Model, VectorDeviceKey: &c.Vector.Device, VectorAPIKey: &c.Vector.APIKey, VectorBaseURL: &c.Vector.BaseURL,
		"account_pool.enabled":                    &c.AccountPool.Enabled,
		"account_pool.strategy":                   &c.AccountPool.Strategy,
		"download.directory_template":             &c.Download.DirectoryTemplate,
		"download.filename_template":              &c.Download.FilenameTemplate,
		"download.path":                           &c.Download.Path,
		"fanbox.auth.default_user_id":             &c.Fanbox.Auth.DefaultUserID,
		"fanbox.flaresolverr.proxy_url":           &c.Fanbox.FlareSolverr.ProxyURL,
		"fanbox.flaresolverr.url":                 &c.Fanbox.FlareSolverr.URL,
		"fanbox.network.proxy_url":                &c.Fanbox.Network.ProxyURL,
		"fanbox.network.user_agent":               &c.Fanbox.Network.UserAgent,
		"logging.format":                          &c.Logging.Format,
		"logging.level":                           &c.Logging.Level,
		"login.open_browser":                      &c.Login.OpenBrowser,
		"login.relay_listen_addr":                 &c.Login.RelayListenAddr,
		"login.relay_public_url":                  &c.Login.RelayPublicURL,
		"login.relay_tls_cert_file":               &c.Login.RelayTLSCertFile,
		"login.relay_tls_key_file":                &c.Login.RelayTLSKeyFile,
		"login.use_after_login":                   &c.Login.UseAfterLogin,
		"mcp.gallery_animation_temp_capacity_mib": &c.MCP.GalleryAnimationTempCapacityMiB,
		"mcp.gallery_cache_capacity_mib":          &c.MCP.GalleryCacheCapacityMiB,
		"network.https_proxy":                     &c.Network.HTTPSProxy,
		"network.request_interval":                &c.Network.RequestInterval,
		"output.json":                             &c.Output.JSON,
		"pixiv.auth.default_user_id":              &c.Pixiv.Auth.DefaultUserID,
		"pixiv.network.proxy_url":                 &c.Pixiv.Network.ProxyURL,
		"reverse_search.flaresolverr.proxy_url":   &c.ReverseSearch.FlareSolverr.ProxyURL,
		"reverse_search.flaresolverr.url":         &c.ReverseSearch.FlareSolverr.URL,
		"reverse_search.network.proxy_url":        &c.ReverseSearch.Network.ProxyURL,
		"reverse_search.network.user_agent":       &c.ReverseSearch.Network.UserAgent,
		"reverse_search.pixiv_only":               &c.ReverseSearch.PixivOnly,
		"reverse_search.provider":                 &c.ReverseSearch.Provider,
		"reverse_search.saucenao_api_key":         &c.ReverseSearch.SauceNAOAPIKey,
		"update.check_enabled":                    &c.Update.CheckEnabled,
	}
}

// Keys 列出 config get/set/unset 接受的 TOML 叶子路径。
func Keys() []string {
	c := Default()
	return slices.Sorted(maps.Keys(fields(&c)))
}

func field(c *Config, key string) (any, error) {
	value, ok := fields(c)[key]
	if !ok {
		return nil, i18n.Errorf("error.unknown_config_key_valid_keys", key, strings.Join(Keys(), ", "))
	}
	return value, nil
}

func text(value any) (string, bool) {
	switch p := value.(type) {
	case *string:
		return *p, true
	case *bool:
		return strconv.FormatBool(*p), true
	case *int64:
		return strconv.FormatInt(*p, 10), true
	case *VectorDevice:
		return string(*p), true
	case *LogLevel:
		return string(*p), true
	case *LogFormat:
		return string(*p), true
	case *AccountPoolStrategy:
		return string(*p), true
	case **string:
		if *p == nil {
			return "", false
		}
		return **p, true
	case **int64:
		if *p == nil {
			return "", false
		}
		return strconv.FormatInt(**p, 10), true
	default:
		panic("unsupported config field")
	}
}

func validateValue(key, raw string) (string, error) {
	switch key {
	case VectorDeviceKey:
		if raw != "" && VectorDevice(raw) != VectorCPU && VectorDevice(raw) != VectorGPU {
			return "", i18n.Errorf("error.must_be_one_of_cpu_gpu", key)
		}
	case "fanbox.flaresolverr.url", "reverse_search.flaresolverr.url":
		if strings.TrimSpace(raw) == "" {
			return "", i18n.Errorf("error.must_be_set", key)
		}
	case "logging.level":
		raw = strings.TrimSpace(raw)
		switch LogLevel(raw) {
		case LogLevelInfo, LogLevelDebug, LogLevelWarn, LogLevelError:
		default:
			return "", i18n.Errorf("error.must_be_one_of_info_debug_warn_error", key)
		}
	case "logging.format":
		raw = strings.TrimSpace(raw)
		switch LogFormat(raw) {
		case LogFormatText, LogFormatJSON:
		default:
			return "", i18n.Errorf("error.must_be_one_of_text_json", key)
		}
	case "reverse_search.provider":
		raw = strings.TrimSpace(raw)
		switch raw {
		case "saucenao", "ascii2d-color", "ascii2d-bovw", "all":
		default:
			return "", i18n.Errorf("error.must_be_one_of_saucenao_ascii2d_color_ascii2d", key)
		}
	case "account_pool.strategy":
		raw = strings.TrimSpace(raw)
		switch AccountPoolStrategy(raw) {
		case AccountPoolStrategyRoundRobin, AccountPoolStrategyRandom:
		default:
			return "", i18n.Errorf("error.must_be_one_of_round_robin_random", key)
		}
	case "network.request_interval":
		interval, err := time.ParseDuration(strings.TrimSpace(raw))
		if err != nil {
			return "", fmt.Errorf("%s: %w", key, err)
		}
		if interval < 0 {
			return "", i18n.Errorf("error.must_not_be_negative", key)
		}
		raw = interval.String()
	case "mcp.gallery_cache_capacity_mib", "mcp.gallery_animation_temp_capacity_mib":
		n, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
		if err != nil {
			return "", i18n.Errorf("error.expects_integer", key, err)
		}
		if n < 0 || n > (1<<63-1)/(1<<20) || (key == "mcp.gallery_animation_temp_capacity_mib" && n == 0) {
			return "", i18n.Errorf("error.is_out_of_range", key)
		}
	case "pixiv.auth.default_user_id", "fanbox.auth.default_user_id":
		n, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
		if err != nil || n <= 0 {
			return "", i18n.Errorf("error.must_be_a_positive_integer", key)
		}
	}
	return raw, nil
}

func validate(c *Config) error {
	// 默认账号在账号选择时校验，普通命令无需有效的账号选择。
	values := fields(c)
	for _, key := range slices.Sorted(maps.Keys(values)) {
		if strings.HasSuffix(key, ".auth.default_user_id") {
			continue
		}
		value, present := text(values[key])
		if present {
			if _, err := validateValue(key, value); err != nil {
				return err
			}
		}
	}
	c.Logging.Level = LogLevel(strings.TrimSpace(string(c.Logging.Level)))
	c.Logging.Format = LogFormat(strings.TrimSpace(string(c.Logging.Format)))
	c.ReverseSearch.Provider = strings.TrimSpace(c.ReverseSearch.Provider)
	c.AccountPool.Strategy = AccountPoolStrategy(strings.TrimSpace(string(c.AccountPool.Strategy)))
	c.Network.RequestInterval = strings.TrimSpace(c.Network.RequestInterval)
	for _, entry := range []struct {
		key    string
		solver FlareSolverrConfig
	}{
		{"fanbox.flaresolverr", c.Fanbox.FlareSolverr},
		{"reverse_search.flaresolverr", c.ReverseSearch.FlareSolverr},
	} {
		if err := validateSolver(entry.key, entry.solver); err != nil {
			return err
		}
	}
	return nil
}

func validateSolver(key string, solver FlareSolverrConfig) error {
	if solver.URL == nil && solver.ProxyURL == nil {
		return nil
	}
	if solver.URL == nil || strings.TrimSpace(*solver.URL) == "" {
		return i18n.Errorf("error.url_must_be_set_when_is_configured", key, key)
	}
	return nil
}
