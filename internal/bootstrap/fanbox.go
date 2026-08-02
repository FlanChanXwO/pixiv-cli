package bootstrap

import (
	"context"
	"os"
	"path/filepath"
	"sync"

	fanboxapp "github.com/FlanChanXwO/pixiv-cli/internal/application/fanbox"
	"github.com/FlanChanXwO/pixiv-cli/internal/platform/localstate"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/authdb"
)

// fanboxDBRegistry 跟踪 NewServices 打开的鉴权数据库，供 CloseServices 在进程
// 退出时统一关闭。每次 NewServices 都打开独立连接，避免测试间共享全局 once。
var (
	fanboxDBRegistryMu sync.Mutex
	fanboxDBRegistry   []*authdb.DB
)

// newFanboxService 打开 appDataDir 下的鉴权数据库、执行 auth.json → SQLite 的
// pixiv legacy 迁移（FANBOX 没有 legacy store），并构造 FANBOX 应用服务。任何
// 本地状态失败都返回 nil：fanbox 命令会给出明确错误，绝不拖垮主 CLI 或 pixiv MCP。
func newFanboxService() *fanboxapp.Service {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	appDataDir := filepath.Join(home, localstate.AppDataDirName)
	db, err := authdb.Open(appDataDir)
	if err != nil {
		return nil
	}
	registerFanboxDB(db)
	// 迁移可安全重入；失败不阻断 FANBOX 服务（pixiv legacy 迁移只影响 pixiv 账号）。
	_, _ = authdb.MigrateLegacyAuthJSON(context.Background(), appDataDir, filepath.Join(appDataDir, "auth.json"))
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
