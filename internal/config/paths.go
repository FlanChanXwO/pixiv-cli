package config

import (
	"errors"
	"io"
	"os"
	"path/filepath"

	constants "github.com/FlanChanXwO/pixiv-cli/internal/platform/localstate"
	"github.com/FlanChanXwO/pixiv-cli/internal/utils/files"
)

const DefaultConfigFileMode = constants.PrivateFileMode

const defaultConfigTemplate = `# pixiv-cli configuration
# Use "pixiv config set KEY VALUE" to change a setting.

[download]
path = "./downloads"
filename_template = "{author} - {title}_{id}"

[web]
fallback_enabled = true

[output]
json = false

[login]
open_browser = true
use_after_login = false

[update]
check_enabled = true
`

var configFilePath = defaultConfigFilePath

func defaultAppConfigDir() (string, error) {
	return files.UserDataSubdir(constants.AppDataDirName)
}

func defaultConfigFilePath() (string, error) {
	return files.UserDataFile(constants.AppDataDirName, "config.toml")
}

func ConfigFilePath() (string, error) {
	return configFilePath()
}

// EnsureDefaultConfigFile 在配置首次缺失时生成只含常用选项的基线文件。高级设置
// （代理、日志、登录超时与 Premium 缓存等）必须由用户显式写入，已有文件绝不覆盖。
func EnsureDefaultConfigFile() error {
	path, err := ConfigFilePath()
	if err != nil {
		return err
	}
	return ensureDefaultConfigFileAt(path)
}

func ensureDefaultConfigFileAt(path string) (err error) {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, constants.PrivateDirMode); err != nil {
		return err
	}
	if err := os.Chmod(directory, constants.PrivateDirMode); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, DefaultConfigFileMode)
	if errors.Is(err, os.ErrExist) {
		return nil
	}
	if err != nil {
		return err
	}
	complete := false
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			err = errors.Join(err, closeErr)
		}
		if !complete {
			if removeErr := os.Remove(path); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
				err = errors.Join(err, removeErr)
			}
		}
	}()
	if err := file.Chmod(DefaultConfigFileMode); err != nil {
		return err
	}
	written, err := io.WriteString(file, defaultConfigTemplate)
	if err != nil {
		return err
	}
	if written != len(defaultConfigTemplate) {
		return io.ErrShortWrite
	}
	if err := file.Sync(); err != nil {
		return err
	}
	complete = true
	return nil
}

func SetFilePathForTest(configPath string) func() {
	oldConfig := configFilePath
	configFilePath = func() (string, error) { return configPath, nil }
	return func() {
		configFilePath = oldConfig
	}
}

func WritePrivateFile(path string, body []byte) error {
	return files.WritePrivateFile(path, body, DefaultConfigFileMode)
}
