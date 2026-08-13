package config

import (
	"errors"
	"os"

	"github.com/FlanChanXwO/pixiv-cli/internal/platform/localstate"
	filesecret "github.com/FlanChanXwO/pixiv-cli/internal/storage/file/secret"
)

// FileStore 是 storage/config 所需的最小文件端口。生产实现由 composition
// root 注入；配置 schema 不直接依赖具体文件系统实现。
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

type defaultFileStore struct{}

// DefaultStore 返回使用 platform/localstate 默认路径与私密文件机制的配置 store。
// 需要注入文件端口的测试或 composition root 应直接构造 Store{Files: ...}。
func DefaultStore() Store { return Store{Files: defaultFileStore{}} }

func (defaultFileStore) Path() (string, error) { return localstate.ConfigFilePath() }

func (defaultFileStore) ReadFile(path string) ([]byte, error) { return os.ReadFile(path) }

func (defaultFileStore) WritePrivateFile(path string, body []byte) error {
	return filesecret.WritePrivateFile(path, body, localstate.PrivateFileMode)
}

func (defaultFileStore) EnsurePrivateFile(path string, body []byte) error {
	return filesecret.EnsurePrivateFile(path, body, localstate.PrivateFileMode)
}

// EnsureDefaultConfigFile 在配置首次缺失时生成只含常用选项的基线文件。高级设置
// （代理、日志、登录超时与 Premium 缓存等）必须由用户显式写入，已有文件绝不覆盖。
func EnsureDefaultConfigFile() error {
	return DefaultStore().EnsureDefaultConfigFile()
}
