// Routes pixiv:// callbacks to the local bridge, an explicit remote handoff,
// or the previously registered URL handler.

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
	LocalRelayURL    string
	RemoteCallback   *RemoteCallbackSession
	RemoteLoginStart *RemoteLoginStart
}

// callbackHandler routes a pixiv:// callback to the active local bridge, an
// explicit remote handoff, or the previous URL handler. Production uses the
// package defaults; tests construct it with recording dependencies.
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
