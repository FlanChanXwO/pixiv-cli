//go:build linux

package secret

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
)

// SecretService 通过 libsecret 提供的 secret-tool 访问当前用户的
// org.freedesktop.secrets 服务。secret-tool 是 Linux 桌面环境常见的
// Secret Service 客户端；它不是项目依赖，缺失时返回明确的不可用错误。
// command 仅供同包测试注入受控的 fake executable。
type SecretService struct {
	command string
}

// GetPassword 读取 Chromium 保存的 libsecret password。application 是固定的
// 浏览器 product name，而不是任意 Secret Service 属性；调用方必须使用
// "chrome" 或 "microsoft-edge"。
func (s SecretService) GetPassword(ctx context.Context, application string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	switch strings.TrimSpace(application) {
	case "chrome", "microsoft-edge":
	default:
		return nil, ErrInvalidItem
	}
	command := s.command
	if command == "" {
		command = "secret-tool"
	}
	if _, err := exec.LookPath(command); err != nil {
		return nil, ErrNotAvailableOnBuild
	}
	// 这些属性与 Chromium 的 chrome_libsecret schema 一致。参数全部来自
	// 上面的固定映射，不能把用户输入拼接进命令行。
	out, err := exec.CommandContext(ctx, command, "lookup", "xdg:schema", "chrome_libsecret", "application", application).Output()
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, ErrSecretService
	}
	password := bytes.TrimRight(out, "\r\n")
	if len(password) == 0 {
		return nil, ErrEmptyPassword
	}
	return append([]byte(nil), password...), nil
}
