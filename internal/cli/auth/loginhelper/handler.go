package loginhelper

import (
	"context"
	"errors"
	"net/url"
	"strings"
)

// CallbackHandlingResult 只携带不含授权码或 token 的本地 bridge URL，或 remote
// relay 返回的一次性最终页 session。callback 本身绝不写入 stdout/stderr。
type CallbackHandlingResult struct {
	LocalRelayURL    string
	RemoteCallback   *RemoteCallbackSession
	RemoteLoginStart *RemoteLoginStart
}

// 以下窄注入点仅隔离 handler 路由测试；生产路径始终使用同文件中的默认实现。
// 测试不能为了验证非白名单委派而真实启动桌面应用。
var (
	callbackRelayURLForHandler            = CallbackRelayURL
	forwardActiveRemoteCallbackForHandler = ForwardActiveRemoteLoginCallback
	delegateToPreviousForHandler          = DelegateToPrevious
)

// HandleCallback 是持久系统协议关联调用的唯一入口。正在进行的本地 login 始终
// 优先；只有没有本地 endpoint 时才尝试由 remote-login 显式建立的一次性 handoff。
// 非白名单 URL 定向交回先前 handler，不会成为任意 pixiv:// URL 的转发器。
func HandleCallback(ctx context.Context, rawURL string) (CallbackHandlingResult, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed == nil || !strings.EqualFold(parsed.Scheme, "pixiv") {
		return CallbackHandlingResult{}, errors.New("invalid Pixiv login link")
	}
	if parsed.Path == remoteLoginLinkPath {
		start, err := ParseRemoteLoginLink(rawURL)
		if err != nil {
			return CallbackHandlingResult{}, err
		}
		return CallbackHandlingResult{RemoteLoginStart: &start}, nil
	}
	if !IsAllowedPixivCallbackURL(rawURL) {
		if err := delegateToPreviousForHandler(ctx, rawURL); err != nil {
			return CallbackHandlingResult{}, err
		}
		return CallbackHandlingResult{}, nil
	}
	localRelayURL, err := callbackRelayURLForHandler(rawURL)
	if err == nil {
		return CallbackHandlingResult{LocalRelayURL: localRelayURL}, nil
	}
	if !errors.Is(err, ErrNoActiveLocalCallback) {
		return CallbackHandlingResult{}, err
	}
	remoteCallback, err := forwardActiveRemoteCallbackForHandler(ctx, rawURL)
	if err == nil {
		return CallbackHandlingResult{RemoteCallback: remoteCallback}, nil
	}
	if !errors.Is(err, ErrNoActiveRemoteLogin) {
		return CallbackHandlingResult{}, err
	}
	if delegateErr := delegateToPreviousForHandler(ctx, rawURL); delegateErr != nil {
		return CallbackHandlingResult{}, delegateErr
	}
	return CallbackHandlingResult{}, nil
}
