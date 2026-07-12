package bootstrap

import (
	"fmt"
	"io"

	"github.com/FlanChanXwO/pixiv-cli/internal/update"
)

// NewUpdateCoordinator 组装显式更新命令使用的生产依赖。
// CLI 仅提供 flags、输出和配置覆盖，更新网络与进程依赖统一由 bootstrap 持有。
func NewUpdateCoordinator(proxy string, out, errOut io.Writer) (*update.UpdateCoordinator, error) {
	httpClient, err := update.NewReleaseHTTPClient(proxy)
	if err != nil {
		return nil, fmt.Errorf("create update HTTP client: %w", err)
	}
	releaseClient, err := update.NewGitHubReleaseClient(update.ReleaseClientOptions{HTTPClient: httpClient})
	if err != nil {
		return nil, fmt.Errorf("create GitHub release client: %w", err)
	}
	return update.NewUpdateCoordinator(update.UpdateCoordinatorOptions{
		SourceDetector:   update.SourceDetectorFunc(update.DetectInstallSource),
		ReleaseChecker:   releaseClient,
		CommandRunner:    update.NewCommandRunner(out, errOut),
		ReleaseInstaller: update.NewReleaseInstaller(productionReleaseInstallerOptions(httpClient)),
	})
}

// NewAutomaticUpdateChecker 组装普通 CLI 命令后的只读更新检查依赖。
// 自动检查不复用显式更新编排器，避免意外执行安装策略或继承其输出语义。
func NewAutomaticUpdateChecker(proxy string) (*update.AutomaticUpdateChecker, error) {
	httpClient, err := update.NewReleaseHTTPClient(proxy)
	if err != nil {
		return nil, fmt.Errorf("create update HTTP client: %w", err)
	}
	releaseClient, err := update.NewGitHubReleaseClient(update.ReleaseClientOptions{HTTPClient: httpClient})
	if err != nil {
		return nil, fmt.Errorf("create GitHub release client: %w", err)
	}
	return update.NewAutomaticUpdateChecker(update.AutomaticUpdateCheckerOptions{
		SourceDetector: update.SourceDetectorFunc(update.DetectInstallSource),
		ReleaseChecker: releaseClient,
	})
}
