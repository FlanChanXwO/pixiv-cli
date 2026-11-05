package secretfile

import (
	"os"

	"github.com/FlanChanXwO/pixiv-cli/internal/config"
)

// ConfigFiles 将配置文件端口接到已有的私有文件写入协议。
type ConfigFiles struct{}

// Path 返回应用管理的配置文件路径。
func (ConfigFiles) Path() (string, error) { return config.ConfigFilePath() }

// ReadFile 读取配置原文。
func (ConfigFiles) ReadFile(path string) ([]byte, error) { return os.ReadFile(path) }

// WritePrivateFile 原子替换配置并保持私有权限。
func (ConfigFiles) WritePrivateFile(path string, body []byte) error {
	return WritePrivateFile(path, body, config.PrivateFileMode)
}

// EnsurePrivateFile 首次创建配置，保留已有文件。
func (ConfigFiles) EnsurePrivateFile(path string, body []byte) error {
	return EnsurePrivateFile(path, body, config.PrivateFileMode)
}
