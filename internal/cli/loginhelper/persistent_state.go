package loginhelper

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	constants "github.com/FlanChanXwO/pixiv-cli/internal/platform/localstate"
	"github.com/FlanChanXwO/pixiv-cli/internal/utils/files"
)

const handlerManifestFilename = "handler-manifest.json"

// handlerManifest 只记录恢复此前系统关联所需的非认证元数据。relay secret
// 永远只在 config.toml 中，不能落入 handler 安装状态。
type handlerManifest struct {
	Version            int                   `json:"version"`
	ExecutablePath     string                `json:"executable_path"`
	HomeDirectory      string                `json:"home_directory,omitempty"`
	PreviousHandler    string                `json:"previous_handler,omitempty"`
	LinuxMIMESnapshots []handlerFileSnapshot `json:"linux_mime_snapshots,omitempty"`
}

// handlerFileSnapshot 是持久 Linux desktop handler 的恢复元数据。它只保存
// mime association 与 desktop entry 文件，绝不承载 OAuth callback、token 或
// relay secret；文件内容仍以私有权限保存在 manifest 内。
type handlerFileSnapshot struct {
	Path    string      `json:"path"`
	Exists  bool        `json:"exists"`
	Mode    os.FileMode `json:"mode"`
	Content []byte      `json:"content,omitempty"`
}

func handlerManifestPath() (string, error) {
	dir, err := files.UserDataSubdir(constants.AppDataDirName)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "url-handler", handlerManifestFilename), nil
}

func loadHandlerManifest() (handlerManifest, bool, error) {
	path, err := handlerManifestPath()
	if err != nil {
		return handlerManifest{}, false, err
	}
	body, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return handlerManifest{}, false, nil
	}
	if err != nil {
		return handlerManifest{}, false, err
	}
	var manifest handlerManifest
	if err := json.Unmarshal(body, &manifest); err != nil || manifest.Version != 1 || manifest.ExecutablePath == "" {
		return handlerManifest{}, false, errors.New("Pixiv URL handler manifest is invalid")
	}
	return manifest, true, nil
}

func saveHandlerManifest(manifest handlerManifest) error {
	if manifest.Version == 0 {
		manifest.Version = 1
	}
	path, err := handlerManifestPath()
	if err != nil {
		return err
	}
	body, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	return files.WritePrivateFile(path, body, constants.PrivateFileMode)
}

func removeHandlerManifest() error {
	path, err := handlerManifestPath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// AutomaticPersistentHandlerSupported 仅对计划中的 desktop client 平台启用启动期
// 注册。desktop Linux 仍由官方 installer/正常 browser login 处理，headless Linux
// server 不会因任意命令尝试建立 XDG 关联。
func AutomaticPersistentHandlerSupported() bool {
	// `go test` 会在同一进程内调用 CLI controller；让测试 binary 注册系统协议
	// 会把真实 desktop association 指向临时测试 executable，既不代表产品行为也
	// 会污染开发环境。测试可通过 CLI 窄注入点覆盖支持条件验证启动逻辑。
	if strings.HasSuffix(filepath.Base(os.Args[0]), ".test") {
		return false
	}
	return runtime.GOOS == "darwin" || runtime.GOOS == "windows"
}

// EnsurePersistentIfNeeded 先以私有 manifest 判断当前 binary 是否已注册；相同
// binary 不重复写系统关联。外部修改 association 时，官方 installer 或 browser
// login 仍会显式重新注册，避免在无关命令中覆盖用户后来选择的默认应用。
func EnsurePersistentIfNeeded(ctx context.Context) error {
	if !AutomaticPersistentHandlerSupported() {
		return nil
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	manifest, exists, err := loadHandlerManifest()
	if err != nil {
		return err
	}
	if exists && filepath.Clean(manifest.ExecutablePath) == filepath.Clean(executable) {
		return nil
	}
	return EnsurePersistent(ctx)
}
