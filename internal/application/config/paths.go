package config

import (
	"errors"
	"io"
	"os"
	"path/filepath"
)

// FileStore 是 application/config 所需的最小文件端口。生产实现由 bootstrap
// 注入 internal/filesystem；配置领域不直接依赖具体文件系统包。
type FileStore interface {
	Path() (string, error)
	ReadFile(string) ([]byte, error)
	WritePrivateFile(string, []byte) error
	EnsurePrivateFile(string, []byte) error
}

func requireFileStore(store FileStore) (FileStore, error) {
	if store == nil {
		return nil, errors.New("config file store is not configured")
	}
	return store, nil
}

const (
	// DefaultConfigFileMode 仅供零值兼容 adapter 使用；生产写入由
	// internal/filesystem 统一执行私密权限和原子替换协议。
	DefaultConfigFileMode os.FileMode = 0o600
	defaultPrivateDirMode os.FileMode = 0o700
)

const defaultConfigTemplate = `# pixiv-cli configuration
# Use "pixiv config set KEY VALUE" to change a setting.

[download]
path = "./downloads"
filename_template = "{author} - {title}_{id}"

[output]
json = false

[login]
open_browser = true
use_after_login = false

[update]
check_enabled = true
`

var configFilePath = defaultConfigFilePath

type defaultFileStore struct{}

func (defaultFileStore) Path() (string, error) { return configFilePath() }

func (defaultFileStore) ReadFile(path string) ([]byte, error) { return os.ReadFile(path) }

func (defaultFileStore) WritePrivateFile(path string, body []byte) error {
	return writePrivateFileFallback(path, body)
}

func (defaultFileStore) EnsurePrivateFile(path string, body []byte) error {
	return ensureDefaultConfigFileAt(path, body)
}

func defaultAppConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".pixiv-cli"), nil
}

func defaultConfigFilePath() (string, error) {
	directory, err := defaultAppConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(directory, "config.toml"), nil
}

func ConfigFilePath() (string, error) {
	return defaultFileStore{}.Path()
}

// EnsureDefaultConfigFile 在配置首次缺失时生成只含常用选项的基线文件。高级设置
// （代理、日志、登录超时与 Premium 缓存等）必须由用户显式写入，已有文件绝不覆盖。
func EnsureDefaultConfigFile() error {
	return (ConfigFileStore{Files: defaultFileStore{}}).EnsureDefaultConfigFile()
}

func ensureDefaultConfigFileAt(path string, body []byte) (err error) {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, defaultPrivateDirMode); err != nil {
		return err
	}
	if err := os.Chmod(directory, defaultPrivateDirMode); err != nil {
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
	written, err := file.Write(body)
	if err != nil {
		return err
	}
	if written != len(body) {
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
	return defaultFileStore{}.WritePrivateFile(path, body)
}

// writePrivateFileFallback 只为保留旧的无参数 helper 兼容性；runtime 生产链
// 通过 FileStore 注入 filesystem.WritePrivateFile，不走该 adapter。
func writePrivateFileFallback(path string, body []byte) (resultErr error) {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, defaultPrivateDirMode); err != nil {
		return err
	}
	if err := os.Chmod(directory, defaultPrivateDirMode); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, ".pixiv-config-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	closed := false
	defer func() {
		if !closed {
			if closeErr := temporary.Close(); closeErr != nil {
				resultErr = errors.Join(resultErr, closeErr)
			}
		}
		if removeErr := os.Remove(temporaryPath); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			resultErr = errors.Join(resultErr, removeErr)
		}
	}()
	if err := temporary.Chmod(DefaultConfigFileMode); err != nil {
		return err
	}
	written, err := temporary.Write(body)
	if err != nil {
		return err
	}
	if written != len(body) {
		return io.ErrShortWrite
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		closed = true
		return err
	}
	closed = true
	if err := os.Rename(temporaryPath, path); err != nil {
		return err
	}
	return nil
}
