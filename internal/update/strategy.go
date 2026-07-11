package update

import (
	"context"
	"fmt"

	"github.com/FlanChanXwO/pixiv-cli/internal/buildinfo"
)

const (
	goInstallPackage      = "github.com/FlanChanXwO/pixiv-cli/cmd/pixiv"
	homebrewStableFormula = "FlanChanXwO/tap/pixiv-cli"
	homebrewBetaFormula   = "FlanChanXwO/tap/pixiv-cli-beta"
)

// SourceDetector 识别当前可执行文件的安装渠道。
type SourceDetector interface {
	Detect(buildinfo.Info) (InstallSource, error)
}

// SourceDetectorFunc 让函数可以作为 SourceDetector 使用。
type SourceDetectorFunc func(buildinfo.Info) (InstallSource, error)

// Detect 调用底层函数。
func (f SourceDetectorFunc) Detect(info buildinfo.Info) (InstallSource, error) {
	return f(info)
}

// ReleaseChecker 查询某个更新渠道可用的 GitHub Release。
type ReleaseChecker interface {
	Check(context.Context, ReleaseCheckOptions) (ReleaseCheckResult, error)
}

// Command 是无需 shell 解析的程序及其参数。
type Command struct {
	Name string
	Args []string
}

// CommandRunner 执行 Homebrew 或 Go 的更新命令。
type CommandRunner interface {
	Run(context.Context, Command) error
}

// ReleaseInstaller 安装已核验的 Release。
type ReleaseInstaller interface {
	Install(context.Context, Release) error
}

// UpdateCoordinatorOptions 注入更新策略需要的外部依赖，供 CLI 和受控测试复用。
type UpdateCoordinatorOptions struct {
	SourceDetector   SourceDetector
	ReleaseChecker   ReleaseChecker
	CommandRunner    CommandRunner
	ReleaseInstaller ReleaseInstaller
}

// UpdateCoordinator 按安装渠道编排显式更新，不负责下载资产细节。
type UpdateCoordinator struct {
	sourceDetector   SourceDetector
	releaseChecker   ReleaseChecker
	commandRunner    CommandRunner
	releaseInstaller ReleaseInstaller
}

// UpdateRequest 描述一次显式 update 命令的用户选择。
type UpdateRequest struct {
	BuildInfo         buildinfo.Info
	Check             bool
	IncludePrerelease bool
}

// UpdateResult 是检查或执行更新后可安全展示的状态。
type UpdateResult struct {
	Source           InstallSource `json:"source"`
	CurrentVersion   string        `json:"current_version"`
	LatestVersion    *string       `json:"latest_version"`
	LatestPrerelease bool          `json:"latest_prerelease"`
	UpdateAvailable  bool          `json:"update_available"`
}

// NewUpdateCoordinator 建立可测试的更新策略编排器。
func NewUpdateCoordinator(options UpdateCoordinatorOptions) (*UpdateCoordinator, error) {
	if options.SourceDetector == nil {
		return nil, fmt.Errorf("update source detector is required")
	}
	if options.ReleaseChecker == nil {
		return nil, fmt.Errorf("update release checker is required")
	}
	if options.ReleaseInstaller == nil {
		options.ReleaseInstaller = NewReleaseInstaller(ReleaseInstallerOptions{})
	}
	return &UpdateCoordinator{
		sourceDetector:   options.SourceDetector,
		releaseChecker:   options.ReleaseChecker,
		commandRunner:    options.CommandRunner,
		releaseInstaller: options.ReleaseInstaller,
	}, nil
}

// Execute 查询更新并在非 check 模式下执行来源对应的更新策略。
func (c *UpdateCoordinator) Execute(ctx context.Context, request UpdateRequest) (UpdateResult, error) {
	if c == nil {
		return UpdateResult{}, fmt.Errorf("update coordinator is nil")
	}
	source, err := c.sourceDetector.Detect(request.BuildInfo)
	if err != nil {
		return UpdateResult{}, fmt.Errorf("detect installation source: %w", err)
	}
	if source == InstallSourceDevelopment {
		return UpdateResult{}, fmt.Errorf("development builds cannot update themselves")
	}
	current, err := parseSemanticVersion(request.BuildInfo.Version)
	if err != nil {
		return UpdateResult{}, fmt.Errorf("parse current build version %q: %w", request.BuildInfo.Version, err)
	}
	checked, err := c.releaseChecker.Check(ctx, ReleaseCheckOptions{
		IncludePrerelease: request.IncludePrerelease,
		Automatic:         false,
	})
	if err != nil {
		return UpdateResult{}, fmt.Errorf("check available releases: %w", err)
	}
	result := UpdateResult{Source: source, CurrentVersion: request.BuildInfo.Version}
	if checked.Release != nil {
		candidate, err := parseSemanticVersion(checked.Release.TagName)
		if err != nil {
			return UpdateResult{}, fmt.Errorf("parse selected GitHub release tag %q: %w", checked.Release.TagName, err)
		}
		latest := checked.Release.TagName
		result.LatestVersion = &latest
		result.LatestPrerelease = checked.Release.Prerelease
		result.UpdateAvailable = candidate.compare(current) > 0
	}
	if homebrewChannelSwitchRequested(source, request.IncludePrerelease) {
		// 渠道切换是用户的显式选择，即使目标渠道当前的 SemVer 较低也不能误报为无需更新。
		result.UpdateAvailable = true
	}
	if request.Check {
		return result, nil
	}
	switch source {
	case InstallSourceHomebrewStable:
		if request.IncludePrerelease {
			return result, c.switchHomebrewFormula(ctx, homebrewStableFormula, homebrewBetaFormula)
		}
		return result, c.runHomebrew(ctx, "upgrade", homebrewStableFormula)
	case InstallSourceHomebrewBeta:
		if request.IncludePrerelease {
			return result, c.runHomebrew(ctx, "upgrade", homebrewBetaFormula)
		}
		return result, c.switchHomebrewFormula(ctx, homebrewBetaFormula, homebrewStableFormula)
	}
	if checked.Release == nil {
		return result, nil
	}
	if !result.UpdateAvailable {
		return result, nil
	}
	switch source {
	case InstallSourceGoInstall:
		if c.commandRunner == nil {
			return UpdateResult{}, fmt.Errorf("go install update command runner is unavailable")
		}
		command := Command{Name: "go", Args: []string{"install", goInstallPackage + "@" + checked.Release.TagName}}
		if err := c.commandRunner.Run(ctx, command); err != nil {
			return UpdateResult{}, fmt.Errorf("run go install update: %w", err)
		}
	case InstallSourceRelease:
		if err := c.releaseInstaller.Install(ctx, *checked.Release); err != nil {
			return UpdateResult{}, fmt.Errorf("install release update %q: %w", checked.Release.TagName, err)
		}
	}
	return result, nil
}

func homebrewChannelSwitchRequested(source InstallSource, includePrerelease bool) bool {
	return (source == InstallSourceHomebrewStable && includePrerelease) || (source == InstallSourceHomebrewBeta && !includePrerelease)
}

func (c *UpdateCoordinator) runHomebrew(ctx context.Context, action, formula string) error {
	if c.commandRunner == nil {
		return fmt.Errorf("Homebrew update command runner is unavailable")
	}
	if err := c.commandRunner.Run(ctx, Command{Name: "brew", Args: []string{action, formula}}); err != nil {
		return fmt.Errorf("Homebrew %s %q: %w", action, formula, err)
	}
	return nil
}

// switchHomebrewFormula 先移除冲突 formula，再安装目标；第二步失败时恢复用户原先的渠道。
func (c *UpdateCoordinator) switchHomebrewFormula(ctx context.Context, previousFormula, targetFormula string) error {
	if err := c.runHomebrew(ctx, "uninstall", previousFormula); err != nil {
		return fmt.Errorf("remove Homebrew formula %q before switching to %q: %w", previousFormula, targetFormula, err)
	}
	installErr := c.runHomebrew(ctx, "install", targetFormula)
	if installErr == nil {
		return nil
	}
	if rollbackErr := c.runHomebrew(ctx, "install", previousFormula); rollbackErr != nil {
		return fmt.Errorf("install Homebrew formula %q after removing %q: %w; rollback reinstall %q failed: %v", targetFormula, previousFormula, installErr, previousFormula, rollbackErr)
	}
	return fmt.Errorf("install Homebrew formula %q after removing %q: %w; rollback reinstall %q succeeded", targetFormula, previousFormula, installErr, previousFormula)
}
