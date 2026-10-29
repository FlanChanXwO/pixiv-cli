package selfupdate

import (
	"fmt"
	"path/filepath"

	"github.com/FlanChanXwO/pixiv-cli/internal/config/paths"
	"github.com/FlanChanXwO/pixiv-cli/internal/selfupdate/installer"
	"github.com/FlanChanXwO/pixiv-cli/internal/selfupdate/release"
)

// 以下别名转发 release 包的公开类型：release 值、渠道策略、缓存端口与 GitHub client。
type (
	Release              = release.Release
	ReleaseCheckOptions  = release.ReleaseCheckOptions
	ReleaseCheckResult   = release.ReleaseCheckResult
	ReleaseChecker       = release.ReleaseChecker
	ReleaseClientOptions = release.ReleaseClientOptions
)

// 以下别名转发 installer 包的安装来源检测与 release 安装器。
type (
	InstallSource           = installer.InstallSource
	SourceDetector          = installer.SourceDetector
	SourceDetectorFunc      = installer.SourceDetectorFunc
	ReleaseInstallerOptions = installer.ReleaseInstallerOptions
	ReleaseInstaller        = installer.ReleaseInstaller
)

// InstallSourceDevelopment 等常量转发 installer 包的安装来源取值。
const (
	InstallSourceDevelopment    = installer.InstallSourceDevelopment
	InstallSourceHomebrewStable = installer.InstallSourceHomebrewStable
	InstallSourceHomebrewBeta   = installer.InstallSourceHomebrewBeta
	InstallSourceGoInstall      = installer.InstallSourceGoInstall
	InstallSourceRelease        = installer.InstallSourceRelease
)

// 这些构造器/函数是 composition root 需要的别名；默认 release cache 由本根组装。
var (
	DetectInstallSource         = installer.DetectInstallSource
	NewReleaseInstaller         = installer.NewReleaseInstaller
	CleanupPendingWindowsUpdate = installer.CleanupPendingWindowsUpdate
	ReleaseSigningKeyID         = installer.ReleaseSigningKeyID
)

// ReleaseSigningPublicKey 保存生产 Release 签名使用的受信 Ed25519 公钥。
var ReleaseSigningPublicKey = installer.ReleaseSigningPublicKey

// NewGitHubReleaseClient 建立 GitHub Releases 查询客户端。未显式注入 Cache 时，
// 使用安装器提供的文件 cache 并落在应用数据目录。
func NewGitHubReleaseClient(options ReleaseClientOptions) (*release.GitHubReleaseClient, error) {
	if options.Cache == nil {
		appDataDir, err := paths.UserDataSubdir(paths.AppDataDirName)
		if err != nil {
			return nil, fmt.Errorf("determine application data directory: %w", err)
		}
		options.Cache = installer.NewFileReleaseCache(filepath.Join(appDataDir, "cache", release.CacheFilename))
	}
	return release.NewGitHubReleaseClient(options)
}
