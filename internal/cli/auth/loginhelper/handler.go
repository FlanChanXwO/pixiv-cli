package loginhelper

import (
	"context"
	"errors"
	"net/url"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/loginrelay"
)

// CallbackHandlingResult 只携带不含授权码或 token 的本地 bridge URL，或 remote
// relay 返回的一次性最终页 session。callback 本身绝不写入 stdout/stderr。
type CallbackHandlingResult struct {
	// LocalRelayURL 会把本次 Pixiv callback 放进 fragment，可能包含授权码；
	// 只有 bridge 的基础地址可以记录，完整值按敏感数据处理。
	LocalRelayURL string
	// RemoteCallback 是远程 relay 返回的一次性结果会话；与 LocalRelayURL 至多一个非零。
	RemoteCallback *RemoteCallbackSession
	// RemoteLoginStart 只在收到 pixiv://remote-login 链接时填写，交由桌面端继续远程登录。
	RemoteLoginStart *RemoteLoginStart
}

// callbackHandler 把 pixiv:// 回调分派给本地 bridge、显式建立的远程 handoff，或先前的
// URL handler。生产代码使用包内默认实现；测试可换成记录型依赖。
type callbackHandler struct {
	relayURL       func(string) (string, error)
	forwardRemote  func(context.Context, string) (*RemoteCallbackSession, error)
	delegateToPrev func(context.Context, string) error
}

func newCallbackHandler() callbackHandler {
	return callbackHandler{
		relayURL:      callbackRelayURL,
		forwardRemote: forwardActiveRemoteLoginCallback,
		delegateToPrev: func(ctx context.Context, rawURL string) error {
			return newURLHandlerInstaller().delegateToPrevious(ctx, rawURL)
		},
	}
}

// HandleCallback 是持久系统协议关联调用的唯一入口。正在进行的本地 login 始终
// 优先；只有没有本地 endpoint 时才尝试由 remote-login 显式建立的一次性 handoff。
// 非白名单 URL 定向交回先前 handler，不会成为任意 pixiv:// URL 的转发器。
func HandleCallback(ctx context.Context, rawURL string) (CallbackHandlingResult, error) {
	return newCallbackHandler().handle(ctx, rawURL)
}

func (h callbackHandler) handle(ctx context.Context, rawURL string) (CallbackHandlingResult, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed == nil || !strings.EqualFold(parsed.Scheme, "pixiv") {
		return CallbackHandlingResult{}, errors.New("invalid Pixiv login link")
	}
	if parsed.Path == remoteLoginLinkPath {
		start, err := parseRemoteLoginLink(rawURL)
		if err != nil {
			return CallbackHandlingResult{}, err
		}
		return CallbackHandlingResult{RemoteLoginStart: &start}, nil
	}
	if !loginrelay.IsAllowedPixivCallbackURL(rawURL) {
		if err := h.delegateToPrev(ctx, rawURL); err != nil {
			return CallbackHandlingResult{}, err
		}
		return CallbackHandlingResult{}, nil
	}
	localRelayURL, err := h.relayURL(rawURL)
	if err == nil {
		return CallbackHandlingResult{LocalRelayURL: localRelayURL}, nil
	}
	if !errors.Is(err, errNoActiveLocalCallback) {
		return CallbackHandlingResult{}, err
	}
	remoteCallback, err := h.forwardRemote(ctx, rawURL)
	if err == nil {
		return CallbackHandlingResult{RemoteCallback: remoteCallback}, nil
	}
	if !errors.Is(err, errNoActiveRemoteLogin) {
		return CallbackHandlingResult{}, err
	}
	if delegateErr := h.delegateToPrev(ctx, rawURL); delegateErr != nil {
		return CallbackHandlingResult{}, delegateErr
	}
	return CallbackHandlingResult{}, nil
}
