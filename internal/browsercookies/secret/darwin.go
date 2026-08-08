//go:build darwin

package secret

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"strings"
)

// Keychain 通过 macOS `security` 命令读取 Keychain generic password item。
type Keychain struct{}

// GetPassword 返回 service/account 对应 item 的密码。输出被严格解析；
// 任何失败的错误都不包含命令输出或密码。
func (Keychain) GetPassword(ctx context.Context, service, account string) ([]byte, error) {
	if strings.TrimSpace(service) == "" || strings.TrimSpace(account) == "" {
		return nil, ErrInvalidItem
	}
	out, err := exec.CommandContext(ctx, "security", "find-generic-password", "-w", "-s", service, "-a", account).Output()
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && strings.Contains(string(exitErr.Stderr), "could not be found") {
			return nil, ErrItemNotFound
		}
		return nil, ErrKeychainCommand
	}
	password := bytes.TrimRight(out, "\r\n")
	if len(password) == 0 {
		return nil, ErrEmptyPassword
	}
	return password, nil
}
