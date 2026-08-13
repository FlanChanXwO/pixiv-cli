package auth

import (
	"errors"
	"net"
	"net/url"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/storage/config"
)

// RelayServerOptions 是 remote Pixiv login relay 的一次性 server 配置。
// ContextWaiterExited 与 Listen 仅供生命周期测试观察/注入内部 ctx watcher 与
// listener；生产路径保持 nil，不能影响 relay 的网络或授权行为。
type RelayServerOptions struct {
	PublicURL           string
	ListenAddr          string
	TLSCertFile         string
	TLSKeyFile          string
	ContextWaiterExited func()
	Listen              func(network, address string) (net.Listener, error)
}

// FlagChangeReader 让 relay 配置选择不依赖 cobra，便于不启动实际 listener 的单测。
type FlagChangeReader interface{ Changed(string) bool }

// ConfiguredRelayServerOptions 按单次 flag > 私有 config 的优先级合成 server
// relay；没有任一 server 值时保持普通本地登录。client target 本身不触发 server。
func ConfiguredRelayServerOptions(cmd FlagChangeReader, opts AccountLoginOptions, cfg config.RuntimeConfig) (RelayServerOptions, bool, error) {
	result := RelayServerOptions{
		PublicURL:   cfg.LoginRelayPublicURL,
		ListenAddr:  cfg.LoginRelayListenAddr,
		TLSCertFile: cfg.LoginRelayTLSCertFile,
		TLSKeyFile:  cfg.LoginRelayTLSKeyFile,
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
		return RelayServerOptions{}, false, nil
	}
	if result.PublicURL == "" || result.ListenAddr == "" {
		return RelayServerOptions{}, false, errors.New("remote login relay requires login_relay_public_url and login_relay_listen_addr")
	}
	if (result.TLSCertFile == "") != (result.TLSKeyFile == "") {
		return RelayServerOptions{}, false, errors.New("remote login relay TLS requires both certificate and key files")
	}
	if err := validateRelayServerOptions(result); err != nil {
		return RelayServerOptions{}, false, err
	}
	return result, true, nil
}

func validateRelayServerOptions(opts RelayServerOptions) error {
	publicURL, err := url.Parse(strings.TrimSpace(opts.PublicURL))
	if err != nil || publicURL == nil || (publicURL.Scheme != "http" && publicURL.Scheme != "https") || publicURL.Host == "" || publicURL.User != nil || publicURL.RawQuery != "" || publicURL.Fragment != "" {
		return errors.New("invalid remote login relay public URL")
	}
	host, _, err := net.SplitHostPort(opts.ListenAddr)
	if err != nil || strings.TrimSpace(host) == "" {
		return errors.New("remote login relay listen address must include host and port")
	}
	if publicURL.Scheme == "https" && opts.TLSCertFile == "" && !isLoopbackHost(host) {
		return errors.New("HTTPS relay without TLS PEM must listen on loopback for a same-host reverse proxy")
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
