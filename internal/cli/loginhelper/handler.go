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
	LocalRelayURL  string
	RemoteCallback *RemoteCallbackSession
}

// 以下窄注入点仅隔离 handler 路由测试；生产路径始终使用同文件中的默认实现。
// 测试不能为了验证非白名单委派而真实启动桌面应用。
var (
	callbackRelayURLForHandler          = CallbackRelayURL
	forwardConfiguredCallbackForHandler = ForwardConfiguredCallback
	delegateToPreviousForHandler        = DelegateToPrevious
)

// HandleCallback 是持久系统协议关联调用的唯一入口。正在进行的本地 login 始终
// 优先；只有没有本地 endpoint 时才尝试远程 relay，非白名单 URL 则定向交回先前
// handler，不会成为任意 pixiv:// URL 的转发器。
func HandleCallback(ctx context.Context, rawURL string) (CallbackHandlingResult, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed == nil || !strings.EqualFold(parsed.Scheme, "pixiv") {
		return CallbackHandlingResult{}, errors.New("invalid Pixiv callback URL")
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
	remoteCallback, err := forwardConfiguredCallbackForHandler(ctx, rawURL)
	if err != nil {
		// 只有明确启用了 remote relay 才抢占 account/login；普通安装但没有
		// client relay 配置时，让原应用继续处理同一路径，避免伤害其他 Pixiv
		// protocol 用户。半配置与上游失败仍显露真实错误，不能静默委派。
		if errors.Is(err, ErrNoConfiguredRelay) {
			if delegateErr := delegateToPreviousForHandler(ctx, rawURL); delegateErr != nil {
				return CallbackHandlingResult{}, delegateErr
			}
			return CallbackHandlingResult{}, nil
		}
		return CallbackHandlingResult{}, err
	}
	return CallbackHandlingResult{RemoteCallback: remoteCallback}, nil
}
