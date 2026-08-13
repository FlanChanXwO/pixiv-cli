package update

import (
	"fmt"
	"path/filepath"

	"github.com/FlanChanXwO/pixiv-cli/internal/platform/localstate"
	"github.com/FlanChanXwO/pixiv-cli/internal/update/installer"
	"github.com/FlanChanXwO/pixiv-cli/internal/update/process"
	"github.com/FlanChanXwO/pixiv-cli/internal/update/release"
)

// re-exports from the release package: release values, channel policy, the
// release cache port, and the GitHub release client.
type (
	Release              = release.Release
	ReleaseAsset         = release.ReleaseAsset
	ReleaseCheckOptions  = release.ReleaseCheckOptions
	ReleaseCheckResult   = release.ReleaseCheckResult
	ReleaseChecker       = release.ReleaseChecker
	GitHubReleaseClient  = release.GitHubReleaseClient
	ReleaseClientOptions = release.ReleaseClientOptions
	ReleaseCache         = release.ReleaseCache
)

// re-exports from the installer package: install-source detection and the
// release installer.
type (
	InstallSource           = installer.InstallSource
	SourceDetector          = installer.SourceDetector
	SourceDetectorFunc      = installer.SourceDetectorFunc
	ReleaseInstaller        = installer.ReleaseInstaller
	ReleaseInstallerOptions = installer.ReleaseInstallerOptions
	ReleaseBinaryChecker    = installer.ReleaseBinaryChecker
	ReleaseFileReplacer     = installer.ReleaseFileReplacer
)

const (
	InstallSourceDevelopment    = installer.InstallSourceDevelopment
	InstallSourceHomebrewStable = installer.InstallSourceHomebrewStable
	InstallSourceHomebrewBeta   = installer.InstallSourceHomebrewBeta
	InstallSourceGoInstall      = installer.InstallSourceGoInstall
	InstallSourceRelease        = installer.InstallSourceRelease
)

// re-exports from the process package: the no-shell command runner.
type (
	Command       = process.Command
	CommandRunner = process.CommandRunner
)

// 这些构造器/函数是 composition root 需要的别名；默认 release cache 由本根组装。
var (
	DetectInstallSource         = installer.DetectInstallSource
	NewReleaseInstaller         = installer.NewReleaseInstaller
	NewCommandRunner            = process.NewCommandRunner
	CleanupPendingWindowsUpdate = installer.CleanupPendingWindowsUpdate
	ReleaseSigningKeyID         = installer.ReleaseSigningKeyID
)

// ReleaseSigningPublicKey 是生产 Release 签名使用的受信 Ed25519 公钥。
var ReleaseSigningPublicKey = installer.ReleaseSigningPublicKey

// NewGitHubReleaseClient 建立 GitHub Releases 查询客户端。未显式注入 Cache 时，
// 使用安装器提供的文件 cache 并落在应用数据目录。
func NewGitHubReleaseClient(options ReleaseClientOptions) (*GitHubReleaseClient, error) {
	if options.Cache == nil {
		cacheDir := options.CacheDir
		if cacheDir == "" {
			appDataDir, err := localstate.UserDataSubdir(localstate.AppDataDirName)
			if err != nil {
				return nil, fmt.Errorf("determine application data directory: %w", err)
			}
			cacheDir = filepath.Join(appDataDir, "cache")
		}
		options.Cache = installer.NewFileReleaseCache(cacheDir, filepath.Join(cacheDir, release.CacheFilename))
	}
	return release.NewGitHubReleaseClient(options)
}
