package auth

import (
	"net"
	"net/url"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/config"
	"github.com/FlanChanXwO/pixiv-cli/internal/i18n"
	"github.com/spf13/pflag"
)

// relayServerOptions 保存 remote Pixiv login relay 的一次性 server 配置。
type relayServerOptions struct {
	PublicURL   string
	ListenAddr  string
	TLSCertFile string
	TLSKeyFile  string
}

// configuredRelayServerOptions 按单次 flag > 私有 config 的优先级合成 server
// relay；没有任一 server 值时保持普通本地登录。client target 本身不触发 server。
func configuredRelayServerOptions(cmd *pflag.FlagSet, opts accountLoginOptions, cfg config.Config) (relayServerOptions, bool, error) {
	result := relayServerOptions{
		PublicURL:   cfg.Login.RelayPublicURL,
		ListenAddr:  cfg.Login.RelayListenAddr,
		TLSCertFile: cfg.Login.RelayTLSCertFile,
		TLSKeyFile:  cfg.Login.RelayTLSKeyFile,
	}
	if cmd.Changed("relay-public-url") {
		result.PublicURL = opts.relayPublicURL
	}
	if cmd.Changed("relay-listen-addr") {
		result.ListenAddr = opts.relayListenAddr
	}
	if cmd.Changed("relay-tls-cert-file") {
		result.TLSCertFile = opts.relayTLSCertFile
	}
	if cmd.Changed("relay-tls-key-file") {
		result.TLSKeyFile = opts.relayTLSKeyFile
	}
	// 历史 relay_secret 与 relay_target_url 可以继续留在用户配置文件中，但它们
	// 不参与模式选择，也不会触发任何网络请求。
	configured := result.PublicURL != "" || result.ListenAddr != "" || result.TLSCertFile != "" || result.TLSKeyFile != ""
	if !configured {
		return relayServerOptions{}, false, nil
	}
	if result.PublicURL == "" || result.ListenAddr == "" {
		return relayServerOptions{}, false, i18n.Errorf("error.remote_login_relay_requires_login_relay_public_url")
	}
	if (result.TLSCertFile == "") != (result.TLSKeyFile == "") {
		return relayServerOptions{}, false, i18n.Errorf("error.remote_login_relay_tls_requires_both_certificate_and")
	}
	if err := validateRelayServerOptions(result); err != nil {
		return relayServerOptions{}, false, err
	}
	return result, true, nil
}

func validateRelayServerOptions(opts relayServerOptions) error {
	publicURL, err := url.Parse(strings.TrimSpace(opts.PublicURL))
	if err != nil || publicURL == nil || (publicURL.Scheme != "http" && publicURL.Scheme != "https") || publicURL.Host == "" || publicURL.User != nil || publicURL.RawQuery != "" || publicURL.Fragment != "" {
		return i18n.Errorf("error.invalid_remote_login_relay_public_url")
	}
	host, _, err := net.SplitHostPort(opts.ListenAddr)
	if err != nil || strings.TrimSpace(host) == "" {
		return i18n.Errorf("error.remote_login_relay_listen_address_must_include_host")
	}
	if publicURL.Scheme == "https" && opts.TLSCertFile == "" && !isLoopbackHost(host) {
		return i18n.Errorf("error.https_relay_without_tls_pem_must_listen_on")
	}
	return nil
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
