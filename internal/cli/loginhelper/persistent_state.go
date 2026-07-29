package loginhelper

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	constants "github.com/FlanChanXwO/pixiv-cli/internal/platform/localstate"
	"github.com/FlanChanXwO/pixiv-cli/internal/utils/files"
)

const handlerManifestFilename = "handler-manifest.json"

// handlerManifest 只记录恢复此前系统关联所需的非认证元数据。relay secret
// 永远只在 config.toml 中，不能落入 handler 安装状态。
type handlerManifest struct {
	Version            int                   `json:"version"`
	ExecutablePath     string                `json:"executable_path"`
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
