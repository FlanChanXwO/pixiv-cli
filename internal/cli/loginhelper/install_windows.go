//go:build windows

package loginhelper

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/common/constants"
)

const defaultWindowsURLHandlerRegistryKey = `HKCU\Software\Classes\pixiv`

var (
	windowsExecutablePath = os.Executable
	runWindowsRegistry    = defaultRunWindowsRegistry
	// tests 使用隔离协议键验证 HKCU 恢复；生产保持 pixiv:// 的固定关联。
	windowsURLHandlerRegistryKey = defaultWindowsURLHandlerRegistryKey
)

// Install 将 pixiv:// 临时关联到当前用户的 pixiv binary。注册表旧树通过 reg
// export/import 精确保存与恢复，因此不会把已有 Pixiv 应用或用户的自定义关联改成
// 长期状态。所有 reg.exe 输出均不透传，避免协议 URL 进入诊断流。
func Install(ctx context.Context, callbackRelayURL string) (cleanup func(), err error) {
	endpointPath, err := writeCallbackEndpoint(callbackRelayURL)
	if err != nil {
		return nil, err
	}
	cleanupEndpoint := func() { _ = os.Remove(endpointPath) }
	installed := false
	defer func() {
		if !installed {
			cleanupEndpoint()
		}
	}()

	executablePath, err := windowsExecutablePath()
	if err != nil {
		return nil, err
	}
	backupDir, err := os.MkdirTemp("", "pixiv-cli-url-handler-registry-*")
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(backupDir, constants.PrivateDirMode); err != nil {
		_ = os.RemoveAll(backupDir)
		return nil, err
	}
	backupPath := filepath.Join(backupDir, "pixiv-url-handler.reg")
	hadPrevious, err := windowsRegistryKeyExists(ctx, windowsURLHandlerRegistryKey)
	if err != nil {
		_ = os.RemoveAll(backupDir)
		return nil, err
	}
	if hadPrevious {
		if err := runWindowsRegistry(ctx, "export", windowsURLHandlerRegistryKey, backupPath, "/y"); err != nil {
			_ = os.RemoveAll(backupDir)
			return nil, err
		}
		if err := os.Chmod(backupPath, constants.PrivateFileMode); err != nil {
			_ = os.RemoveAll(backupDir)
			return nil, err
		}
	}

	restore := func() {
		_ = runWindowsRegistry(context.Background(), "delete", windowsURLHandlerRegistryKey, "/f")
		if hadPrevious {
			_ = runWindowsRegistry(context.Background(), "import", backupPath)
		}
		_ = os.RemoveAll(backupDir)
	}
	if err := installWindowsURLHandler(ctx, executablePath); err != nil {
		restore()
		return nil, err
	}

	installed = true
	return func() {
		cleanupEndpoint()
		restore()
	}, nil
}

func defaultRunWindowsRegistry(ctx context.Context, args ...string) error {
	command := exec.CommandContext(ctx, "reg.exe", args...)
	if err := command.Run(); err != nil {
		return fmt.Errorf("run Windows registry command: %w", err)
	}
	return nil
}

func windowsRegistryKeyExists(ctx context.Context, key string) (bool, error) {
	err := runWindowsRegistry(ctx, "query", key)
	if err == nil {
		return true, nil
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		return false, nil
	}
	return false, err
}

func installWindowsURLHandler(ctx context.Context, executablePath string) error {
	command := windowsURLHandlerCommand(executablePath)
	if err := runWindowsRegistry(ctx, "add", windowsURLHandlerRegistryKey, "/ve", "/t", "REG_SZ", "/d", "URL:Pixiv Protocol", "/f"); err != nil {
		return err
	}
	if err := runWindowsRegistry(ctx, "add", windowsURLHandlerRegistryKey, "/v", "URL Protocol", "/t", "REG_SZ", "/d", "", "/f"); err != nil {
		return err
	}
	return runWindowsRegistry(ctx, "add", windowsURLHandlerRegistryKey+`\shell\open\command`, "/ve", "/t", "REG_SZ", "/d", command, "/f")
}

// windowsURLHandlerCommand 由 reg.exe 作为单个值写入，随后由 Shell 直接解析；
// 因此只对双引号做 Windows command-line 转义，不引入 cmd.exe 或 shell 拼接。
func windowsURLHandlerCommand(executablePath string) string {
	escaped := strings.ReplaceAll(executablePath, `"`, `\"`)
	return `"` + escaped + `" auth ` + CallbackCommand + ` "%1"`
}
