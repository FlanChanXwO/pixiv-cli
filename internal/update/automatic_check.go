package update

import (
	"context"
	"fmt"

	"github.com/FlanChanXwO/pixiv-cli/internal/shared/buildinfo"
	"github.com/FlanChanXwO/pixiv-cli/internal/update/release"
)

// AutomaticUpdateCheckerOptions 注入自动检查所需的只读依赖。
// 自动检查不执行安装命令，也不接触 Release 资产。
type AutomaticUpdateCheckerOptions struct {
	SourceDetector SourceDetector
	ReleaseChecker ReleaseChecker
}

// AutomaticUpdateChecker 在普通 CLI 命令成功后判断是否应提示稳定版更新。
type AutomaticUpdateChecker struct {
	sourceDetector SourceDetector
	releaseChecker ReleaseChecker
}

// AutomaticUpdateRequest 描述当前二进制的构建信息。
type AutomaticUpdateRequest struct {
	BuildInfo buildinfo.Info
}

// AutomaticUpdateNotice 是 CLI 可安全展示的更新提示。
// UpdateCommand 是安装来源对应的准确后续命令，不会在检查阶段执行。
type AutomaticUpdateNotice struct {
	Source         InstallSource
	CurrentVersion string
	LatestVersion  string
	UpdateCommand  string
}

// NewAutomaticUpdateChecker 建立自动更新检查器。
func NewAutomaticUpdateChecker(options AutomaticUpdateCheckerOptions) (*AutomaticUpdateChecker, error) {
	if options.SourceDetector == nil {
		return nil, fmt.Errorf("automatic update source detector is required")
	}
	if options.ReleaseChecker == nil {
		return nil, fmt.Errorf("automatic update release checker is required")
	}
	return &AutomaticUpdateChecker{
		sourceDetector: options.SourceDetector,
		releaseChecker: options.ReleaseChecker,
	}, nil
}

// Check 查询稳定版更新，并仅在较新版本存在时返回提示。
// 开发构建没有发布渠道，必须在读取本机安装来源或访问网络前返回。
func (c *AutomaticUpdateChecker) Check(ctx context.Context, request AutomaticUpdateRequest) (*AutomaticUpdateNotice, error) {
	if c == nil {
		return nil, fmt.Errorf("automatic update checker is nil")
	}
	if request.BuildInfo.IsDevelopment() {
		return nil, nil
	}
	current, err := release.ParseSemanticVersion(request.BuildInfo.Version)
	if err != nil {
		return nil, fmt.Errorf("parse current build version %q: %w", request.BuildInfo.Version, err)
	}
	checked, err := c.releaseChecker.Check(ctx, ReleaseCheckOptions{
		Automatic:         true,
		IncludePrerelease: false,
	})
	if err != nil {
		return nil, fmt.Errorf("check available releases: %w", err)
	}
	// GitHub client 的节流代表此前已经完成过一次检查；不得重复展示同一提示。
	if checked.Throttled || checked.Release == nil {
		return nil, nil
	}
	candidate, err := release.ParseSemanticVersion(checked.Release.TagName)
	if err != nil {
		return nil, fmt.Errorf("parse selected GitHub release tag %q: %w", checked.Release.TagName, err)
	}
	if candidate.Compare(current) <= 0 {
		return nil, nil
	}
	source, err := c.sourceDetector.Detect(request.BuildInfo)
	if err != nil {
		return nil, fmt.Errorf("detect installation source: %w", err)
	}
	command, err := automaticUpdateCommand(source, checked.Release.TagName)
	if err != nil {
		return nil, err
	}
	if source == InstallSourceDevelopment {
		return nil, nil
	}
	return &AutomaticUpdateNotice{
		Source:         source,
		CurrentVersion: request.BuildInfo.Version,
		LatestVersion:  checked.Release.TagName,
		UpdateCommand:  command,
	}, nil
}

func automaticUpdateCommand(source InstallSource, tag string) (string, error) {
	switch source {
	case InstallSourceHomebrewStable:
		return "brew upgrade " + homebrewStableFormula, nil
	case InstallSourceHomebrewBeta:
		// 自动检查只选择 stable；复用显式策略以安全切换 beta formula 回 stable。
		return "pixiv update", nil
	case InstallSourceGoInstall:
		return "go install " + goInstallPackage + "@" + tag, nil
	case InstallSourceRelease:
		return "pixiv update", nil
	case InstallSourceDevelopment:
		return "", nil
	default:
		return "", fmt.Errorf("unsupported installation source %q for automatic update", source)
	}
}
