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

	"github.com/FlanChanXwO/pixiv-cli/internal/platform/localstate"
)

const defaultWindowsURLHandlerRegistryKey = `HKCU\Software\Classes\pixiv`

// PreviousWindowsURLHandlerProgID 是保存接管前 registry tree 的私有 ProgID。
const PreviousWindowsURLHandlerProgID = `pixiv-cli.previous-pixiv`
const previousWindowsURLHandlerRegistryKey = `HKCU\Software\Classes\` + PreviousWindowsURLHandlerProgID

var (
	windowsExecutablePath    = os.Executable
	runWindowsRegistry       = defaultRunWindowsRegistry
	openWindowsPreviousClass = windowsShellOpenClass
	// tests 使用隔离协议键验证 HKCU 恢复；生产保持 pixiv:// 的固定关联。
	windowsURLHandlerRegistryKey = defaultWindowsURLHandlerRegistryKey
)

// SetWindowsExecutablePath 覆盖本机 executable 解析；返回恢复函数。
func SetWindowsExecutablePath(resolve func() (string, error)) func() {
	original := windowsExecutablePath
	windowsExecutablePath = resolve
	return func() { windowsExecutablePath = original }
}

// SetRunWindowsRegistry 覆盖 reg.exe 命令执行；返回恢复函数。
func SetRunWindowsRegistry(run func(context.Context, ...string) error) func() {
	original := runWindowsRegistry
	runWindowsRegistry = run
	return func() { runWindowsRegistry = original }
}

// SetOpenWindowsPreviousClass 覆盖先前 handler 的 ShellExecuteEx 启动；返回恢复函数。
func SetOpenWindowsPreviousClass(open func(context.Context, string, string) error) func() {
	original := openWindowsPreviousClass
	openWindowsPreviousClass = open
	return func() { openWindowsPreviousClass = original }
}

// SetWindowsURLHandlerRegistryKey 覆盖测试使用的隔离协议注册表键；返回恢复函数。
func SetWindowsURLHandlerRegistryKey(key string) func() {
	original := windowsURLHandlerRegistryKey
	windowsURLHandlerRegistryKey = key
	return func() { windowsURLHandlerRegistryKey = original }
}

// EnsurePersistent 建立当前用户范围的长期协议关联，并在第一次接管时把完整旧
// registry tree 复制到私有 ProgID。manifest 不含 remote relay secret。
func EnsurePersistent(ctx context.Context) error {
	executablePath, err := windowsExecutablePath()
	if err != nil {
		return err
	}
	manifest, exists, err := LoadHandlerManifest()
	if err != nil {
		return err
	}
	if !exists {
		hadPrevious, err := windowsRegistryKeyExists(ctx, windowsURLHandlerRegistryKey)
		if err != nil {
			return err
		}
		manifest = HandlerManifest{Version: 1, ExecutablePath: executablePath}
		if hadPrevious {
			if err := runWindowsRegistry(ctx, "copy", windowsURLHandlerRegistryKey, previousWindowsURLHandlerRegistryKey, "/s", "/f"); err != nil {
				return err
			}
			// ShellExecuteEx 的 class 参数是 ProgID，而非完整 HKCU registry
			// 路径；复制仍使用完整键，manifest 则只保存可启动的 class。
			manifest.PreviousHandler = PreviousWindowsURLHandlerProgID
		}
	} else {
		manifest.ExecutablePath = executablePath
	}
	if err := installWindowsURLHandler(ctx, executablePath); err != nil {
		return err
	}
	if err := SaveHandlerManifest(manifest); err != nil {
		return err
	}
	return nil
}

func DisablePersistent(ctx context.Context) error {
	manifest, exists, err := LoadHandlerManifest()
	if err != nil || !exists {
		return err
	}
	ours, err := windowsDefaultHandlerIsOurs(ctx)
	if err != nil {
		return err
	}
	if ours {
		if err := runWindowsRegistry(ctx, "delete", windowsURLHandlerRegistryKey, "/f"); err != nil {
			return err
		}
		if manifest.PreviousHandler != "" {
			if err := runWindowsRegistry(ctx, "copy", previousWindowsURLHandlerRegistryKey, windowsURLHandlerRegistryKey, "/s", "/f"); err != nil {
				return err
			}
		}
	}
	if manifest.PreviousHandler != "" {
		if err := runWindowsRegistry(ctx, "delete", previousWindowsURLHandlerRegistryKey, "/f"); err != nil {
			return err
		}
	}
	return RemoveHandlerManifest()
}

// DelegateToPrevious 在 Windows 使用明确 class 的 ShellExecuteExW 启动先前
// protocol class，不让 callback URL 进入 cmd.exe 或字符串拼接。
func DelegateToPrevious(ctx context.Context, rawURL string) error {
	manifest, exists, err := LoadHandlerManifest()
	if err != nil {
		return err
	}
	if !exists || manifest.PreviousHandler == "" {
		return errors.New("no previous Pixiv URL handler is available")
	}
	if err := openWindowsPreviousClass(ctx, manifest.PreviousHandler, rawURL); err != nil {
		return errors.New("could not open previous Pixiv URL handler")
	}
	return nil
}

// Install 将 pixiv:// 临时关联到当前用户的 pixiv binary。注册表旧树通过 reg
// export/import 精确保存与恢复，因此不会把已有 Pixiv 应用或用户的自定义关联改成
// 长期状态。所有 reg.exe 输出均不透传，避免协议 URL 进入诊断流。
func Install(ctx context.Context, callbackRelayURL string) (cleanup func(), err error) {
	endpointPath, err := WriteCallbackEndpoint(callbackRelayURL)
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
	if err := os.Chmod(backupDir, localstate.PrivateDirMode); err != nil {
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
		if err := os.Chmod(backupPath, localstate.PrivateFileMode); err != nil {
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
	command := WindowsURLHandlerCommand(executablePath)
	if err := runWindowsRegistry(ctx, "add", windowsURLHandlerRegistryKey, "/ve", "/t", "REG_SZ", "/d", "URL:Pixiv Protocol", "/f"); err != nil {
		return err
	}
	if err := runWindowsRegistry(ctx, "add", windowsURLHandlerRegistryKey, "/v", "URL Protocol", "/t", "REG_SZ", "/d", "", "/f"); err != nil {
		return err
	}
	return runWindowsRegistry(ctx, "add", windowsURLHandlerRegistryKey+`\shell\open\command`, "/ve", "/t", "REG_SZ", "/d", command, "/f")
}

func windowsDefaultHandlerIsOurs(ctx context.Context) (bool, error) {
	command := exec.CommandContext(ctx, "reg.exe", "query", windowsURLHandlerRegistryKey+`\shell\open\command`, "/ve")
	out, err := command.Output()
	if err != nil {
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			return false, nil
		}
		return false, err
	}
	return strings.Contains(strings.ToLower(string(out)), " auth "+CallbackCommand+" "), nil
}

// WindowsURLHandlerCommand 由 reg.exe 作为单个值写入，随后由 Shell 直接解析；
// 因此只对双引号做 Windows command-line 转义，不引入 cmd.exe 或 shell 拼接。
func WindowsURLHandlerCommand(executablePath string) string {
	escaped := strings.ReplaceAll(executablePath, `"`, `\"`)
	return `"` + escaped + `" auth ` + CallbackCommand + ` "%1"`
}
