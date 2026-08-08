package bootstrap

import (
	"os"

	configapp "github.com/FlanChanXwO/pixiv-cli/internal/application/config"
	"github.com/FlanChanXwO/pixiv-cli/internal/filesystem"
)

// filesystemConfigFiles 把配置 application port 绑定到统一 filesystem 原语。
// 路径方法保留 config.ConfigFilePath 的测试覆写 seam；生产默认路径仍由
// filesystem 的用户数据命名空间决定。
type filesystemConfigFiles struct{}

func (filesystemConfigFiles) Path() (string, error) { return configapp.ConfigFilePath() }

func (filesystemConfigFiles) ReadFile(path string) ([]byte, error) { return os.ReadFile(path) }

func (filesystemConfigFiles) WritePrivateFile(path string, body []byte) error {
	return filesystem.WritePrivateFile(path, body, filesystem.PrivateFileMode)
}

func (filesystemConfigFiles) EnsurePrivateFile(path string, body []byte) error {
	return filesystem.EnsurePrivateFile(path, body, filesystem.PrivateFileMode)
}

func runtimeConfigFileStore() configapp.ConfigFileStore {
	return configapp.ConfigFileStore{Files: filesystemConfigFiles{}}
}

// ConfigFilePath 返回由 runtime 使用的配置路径，不构造 authdb 或其他服务。
func ConfigFilePath() (string, error) { return runtimeConfigFileStore().Path() }
