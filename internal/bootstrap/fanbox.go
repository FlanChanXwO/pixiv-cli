package bootstrap

import (
	configapp "github.com/FlanChanXwO/pixiv-cli/internal/application/config"
	fanboxapp "github.com/FlanChanXwO/pixiv-cli/internal/application/fanbox"
	"github.com/FlanChanXwO/pixiv-cli/internal/persistence/authdb"
	fanboxsdk "github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
)

// newFanboxService 使用 Runtime 已打开的鉴权数据库构造 FANBOX 应用服务。
// db 为 nil 时返回 nil：fanbox 命令会给出明确错误，绝不拖垮主 CLI 或 pixiv MCP。
func newFanboxService(db *authdb.DB, store configapp.ConfigFileStore) *fanboxapp.Service {
	if db == nil {
		return nil
	}
	service := fanboxapp.New(fanboxPersistenceAdapter{db: db}, configDefaultStore{store: store})
	service.LoadOptionsFunc = func() (fanboxsdk.Options, error) {
		cfg, err := LoadRuntimeConfig()
		if err != nil {
			return fanboxsdk.Options{}, err
		}
		options := fanboxsdk.Options{}
		if cfg.FanboxNetwork.ProxyURL.Present {
			options.ProxyURL = cfg.FanboxNetwork.ProxyURL.Value
		} else {
			options.ProxyURL = cfg.HTTPSProxy
		}
		if cfg.FanboxNetwork.UserAgent.Present {
			options.UserAgent = cfg.FanboxNetwork.UserAgent.Value
		}
		if cfg.FanboxFlareSolverr != nil {
			options.FlareSolverr = &fanboxsdk.FlareSolverrOptions{
				URL:      cfg.FanboxFlareSolverr.URL,
				ProxyURL: cfg.FanboxFlareSolverr.ProxyURL,
			}
		}
		return options, nil
	}
	return service
}
