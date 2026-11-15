package session

import (
	"context"
	"errors"

	"github.com/FlanChanXwO/pixiv-cli/internal/fanbox/report"
	"github.com/FlanChanXwO/pixiv-cli/internal/lifecycle"
	"github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
)

// ProxyRequest 保存一次 Pixiv 数据执行的账号与代理覆盖。代理到 SDK Options 的翻译
// 由组合根负责；本包不读取配置或 flag。
type ProxyRequest struct {
	UserID             int64
	HTTPSProxyOverride *string
}

// Open 打开延迟构造的 FANBOX 会话的默认账号租约；session 延迟解析，构造命令树时不打开账号库。
func Open(ctx context.Context, open func(context.Context) (*Fanbox, error), request OpenRequest) (*lifecycle.Lease[*fanbox.Client], error) {
	service, err := open(ctx)
	if err != nil {
		return nil, err
	}
	return service.Open(ctx, request)
}

// Use 让租约只覆盖一次命令回调；回调和关闭的错误都会保留并合并返回。
func Use(ctx context.Context, open func(context.Context) (*Fanbox, error), request OpenRequest, use func(*fanbox.Client) error) error {
	lease, err := Open(ctx, open, request)
	if err != nil {
		return report.Wrap(err, "")
	}
	useErr := use(lease.Value())
	return errors.Join(report.Wrap(useErr, ""), lease.Close())
}
