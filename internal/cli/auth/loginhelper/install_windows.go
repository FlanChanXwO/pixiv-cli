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

	"github.com/FlanChanXwO/pixiv-cli/internal/config/paths"
)

const defaultWindowsURLHandlerRegistryKey = `HKCU\Software\Classes\pixiv`

// previousWindowsURLHandlerProgID 指定保存接管前 registry tree 的私有 ProgID。
const previousWindowsURLHandlerProgID = `pixiv-cli.previous-pixiv`
const previousWindowsURLHandlerRegistryKey = `HKCU\Software\Classes\` + previousWindowsURLHandlerProgID

// urlHandlerInstaller 持有 Windows 注册表与 shell 边界。生产代码经 newURLHandlerInstaller
// 构造；测试使用隔离的注册表键和记录型命令。
type urlHandlerInstaller struct {
	executable    func() (string, error)
	runRegistry   func(context.Context, ...string) error
	openPrevious  func(context.Context, string, string) error
	urlHandlerKey string
}

func newURLHandlerInstaller() urlHandlerInstaller {
	return urlHandlerInstaller{
		executable:    os.Executable,
		runRegistry:   defaultRunWindowsRegistry,
		openPrevious:  windowsShellOpenClass,
		urlHandlerKey: defaultWindowsURLHandlerRegistryKey,
	}
}

// ensurePersistent 建立当前用户范围的长期协议关联，并在第一次接管时把完整旧
// registry tree 复制到私有 ProgID。manifest 不含 remote relay secret。
func (i urlHandlerInstaller) ensurePersistent(ctx context.Context) error {
	executablePath, err := i.executable()
	if err != nil {
		return err
	}
	manifest, exists, err := loadHandlerManifest()
	if err != nil {
		return err
	}
	if !exists {
		hadPrevious, err := i.registryKeyExists(ctx, i.urlHandlerKey)
		if err != nil {
			return err
		}
		manifest = handlerManifest{Version: 1, ExecutablePath: executablePath}
		if hadPrevious {
			if err := i.runRegistry(ctx, "copy", i.urlHandlerKey, previousWindowsURLHandlerRegistryKey, "/s", "/f"); err != nil {
				return err
			}
			// ShellExecuteEx 的 class 参数是 ProgID，而非完整 HKCU registry
			// 路径；复制仍使用完整键，manifest 则只保存可启动的 class。
			manifest.PreviousHandler = previousWindowsURLHandlerProgID
		}
	} else {
		manifest.ExecutablePath = executablePath
	}
	if err := i.installURLHandler(ctx, executablePath); err != nil {
		return err
	}
	if err := saveHandlerManifest(manifest); err != nil {
		return err
	}
	return nil
}

// delegateToPrevious 在 Windows 使用明确 class 的 ShellExecuteExW 启动先前
// protocol class，不让 callback URL 进入 cmd.exe 或字符串拼接。
func (i urlHandlerInstaller) delegateToPrevious(ctx context.Context, rawURL string) error {
	manifest, exists, err := loadHandlerManifest()
	if err != nil {
		return err
	}
	if !exists || manifest.PreviousHandler == "" {
		return errors.New("no previous Pixiv URL handler is available")
	}
	if err := i.openPrevious(ctx, manifest.PreviousHandler, rawURL); err != nil {
		return errors.New("could not open previous Pixiv URL handler")
	}
	return nil
}

// install 将 pixiv:// 临时关联到当前用户的 pixiv binary。注册表旧树通过 reg
// export/import 精确保存与恢复，因此不会把已有 Pixiv 应用或用户的自定义关联改成
// 长期状态。所有 reg.exe 输出均不透传，避免协议 URL 进入诊断流。
func (i urlHandlerInstaller) install(ctx context.Context, callbackRelayURL string) (cleanup func(), err error) {
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

	executablePath, err := i.executable()
	if err != nil {
		return nil, err
	}
	backupDir, err := os.MkdirTemp("", "pixiv-cli-url-handler-registry-*")
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(backupDir, paths.PrivateDirMode); err != nil {
		_ = os.RemoveAll(backupDir)
		return nil, err
	}
	backupPath := filepath.Join(backupDir, "pixiv-url-handler.reg")
	hadPrevious, err := i.registryKeyExists(ctx, i.urlHandlerKey)
	if err != nil {
		_ = os.RemoveAll(backupDir)
		return nil, err
	}
	if hadPrevious {
		if err := i.runRegistry(ctx, "export", i.urlHandlerKey, backupPath, "/y"); err != nil {
			_ = os.RemoveAll(backupDir)
			return nil, err
		}
		if err := os.Chmod(backupPath, paths.PrivateFileMode); err != nil {
			_ = os.RemoveAll(backupDir)
			return nil, err
		}
	}

	restore := func() {
		_ = i.runRegistry(context.Background(), "delete", i.urlHandlerKey, "/f")
		if hadPrevious {
			_ = i.runRegistry(context.Background(), "import", backupPath)
		}
		_ = os.RemoveAll(backupDir)
	}
	if err := i.installURLHandler(ctx, executablePath); err != nil {
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

func (i urlHandlerInstaller) registryKeyExists(ctx context.Context, key string) (bool, error) {
	err := i.runRegistry(ctx, "query", key)
	if err == nil {
		return true, nil
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		return false, nil
	}
	return false, err
}

func (i urlHandlerInstaller) installURLHandler(ctx context.Context, executablePath string) error {
	command := windowsURLHandlerCommand(executablePath)
	if err := i.runRegistry(ctx, "add", i.urlHandlerKey, "/ve", "/t", "REG_SZ", "/d", "URL:Pixiv Protocol", "/f"); err != nil {
		return err
	}
	if err := i.runRegistry(ctx, "add", i.urlHandlerKey, "/v", "URL Protocol", "/t", "REG_SZ", "/d", "", "/f"); err != nil {
		return err
	}
	return i.runRegistry(ctx, "add", i.urlHandlerKey+`\shell\open\command`, "/ve", "/t", "REG_SZ", "/d", command, "/f")
}

// windowsURLHandlerCommand 由 reg.exe 作为单个值写入，随后由 Shell 直接解析；
// 因此只对双引号做 Windows command-line 转义，不引入 cmd.exe 或 shell 拼接。
func windowsURLHandlerCommand(executablePath string) string {
	escaped := strings.ReplaceAll(executablePath, `"`, `\"`)
	return `"` + escaped + `" auth ` + CallbackCommand + ` "%1"`
}

// Install 将 pixiv:// 临时关联到当前用户的 pixiv binary。
func Install(ctx context.Context, callbackRelayURL string) (func(), error) {
	return newURLHandlerInstaller().install(ctx, callbackRelayURL)
}
