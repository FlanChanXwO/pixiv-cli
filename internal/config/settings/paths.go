package settings

import (
	"errors"
	"os"

	"github.com/FlanChanXwO/pixiv-cli/internal/config/paths"
	filesecret "github.com/FlanChanXwO/pixiv-cli/internal/storage/file/secret"
)

// FileStore 是 config/settings 所需的最小文件端口。生产实现由 composition
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

type defaultFileStore struct{}

// DefaultStore 返回使用 config/paths 默认路径与私密文件机制的配置 store。
// 需要注入文件端口的测试或 CLI composition root 应直接构造 Store{Files: ...}。
func DefaultStore() Store { return Store{Files: defaultFileStore{}} }

func (defaultFileStore) Path() (string, error) { return paths.ConfigFilePath() }

func (defaultFileStore) ReadFile(path string) ([]byte, error) { return os.ReadFile(path) }

func (defaultFileStore) WritePrivateFile(path string, body []byte) error {
	return filesecret.WritePrivateFile(path, body, paths.PrivateFileMode)
}

func (defaultFileStore) EnsurePrivateFile(path string, body []byte) error {
	return filesecret.EnsurePrivateFile(path, body, paths.PrivateFileMode)
}

// EnsureDefaultConfigFile 在配置首次缺失时生成由 schema 元数据驱动的精简基线文件。
// 高级设置仍需由用户显式写入，已有文件绝不覆盖。
func EnsureDefaultConfigFile() error {
	return DefaultStore().EnsureDefaultConfigFile()
}
