package bootstrap

import (
	"sync"

	fanboxapp "github.com/FlanChanXwO/pixiv-cli/internal/application/fanbox"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/authdb"
)

// fanboxDBRegistry 跟踪 NewServices 打开的鉴权数据库，供 CloseServices 在进程
// 退出时统一关闭。每次 NewServices 都打开独立连接，避免测试间共享全局 once。
var (
	fanboxDBRegistryMu sync.Mutex
	fanboxDBRegistry   []*authdb.DB
)

// newFanboxService 使用 NewServices 已打开的鉴权数据库构造 FANBOX 应用服务。
// db 为 nil 时返回 nil：fanbox 命令会给出明确错误，绝不拖垮主 CLI 或 pixiv MCP。
func newFanboxService(db *authdb.DB, appDataDir string) *fanboxapp.Service {
	if db == nil {
		return nil
	}
	return fanboxapp.New(db, appDataDir)
}

func registerFanboxDB(db *authdb.DB) {
	fanboxDBRegistryMu.Lock()
	fanboxDBRegistry = append(fanboxDBRegistry, db)
	fanboxDBRegistryMu.Unlock()
}

// CloseServices 关闭 NewServices 打开的全部本地数据库。
func CloseServices() {
	fanboxDBRegistryMu.Lock()
	dbs := fanboxDBRegistry
	fanboxDBRegistry = nil
	fanboxDBRegistryMu.Unlock()
	for _, db := range dbs {
		_ = db.Close()
	}
}
