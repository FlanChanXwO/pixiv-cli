package update

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"

	"github.com/FlanChanXwO/pixiv-cli/internal/utils/uri"
)

// NewReleaseHTTPClient 建立只供更新查询使用的 HTTP client。
// 显式 update 的网络生命周期由调用方 context 控制，不能复用 Pixiv API 的固定超时。
func NewReleaseHTTPClient(proxyURL string) (*http.Client, error) {
	transport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return nil, fmt.Errorf("default HTTP transport has unexpected type %T", http.DefaultTransport)
	}
	clonedTransport := transport.Clone()
	// 配置层已处理 https_proxy/HTTPS_PROXY 优先级；这里禁用隐式环境回退，确保 --proxy 可预测地覆盖配置。
	clonedTransport.Proxy = nil
	if proxyURL != "" {
		parsedProxy, err := url.ParseRequestURI(proxyURL)
		if err != nil {
			return nil, fmt.Errorf("parse update proxy URL: %w", uri.ErrInvalidProxy)
		}
		if (parsedProxy.Scheme != "http" && parsedProxy.Scheme != "https" && parsedProxy.Scheme != "socks5" && parsedProxy.Scheme != "socks5h") || parsedProxy.Host == "" {
			return nil, fmt.Errorf("update proxy URL must use http, https, socks5, or socks5h: %w", uri.ErrInvalidProxy)
		}
		clonedTransport.Proxy = http.ProxyURL(parsedProxy)
	}
	return &http.Client{Transport: clonedTransport}, nil
}

// NewCommandRunner 建立把子进程输出直连到 CLI 输出流的无 shell 命令执行器。
func NewCommandRunner(out, errOut io.Writer) CommandRunner {
	return processCommandRunner{out: out, errOut: errOut}
}

type processCommandRunner struct {
	out    io.Writer
	errOut io.Writer
}

func (r processCommandRunner) Run(ctx context.Context, command Command) error {
	if command.Name == "" {
		return fmt.Errorf("command name is empty")
	}
	process := exec.CommandContext(ctx, command.Name, command.Args...)
	process.Stdout = r.out
	process.Stderr = r.errOut
	if err := process.Run(); err != nil {
		return fmt.Errorf("run command %q: %w", command.Name, err)
	}
	return nil
}
