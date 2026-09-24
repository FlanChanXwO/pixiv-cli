// Package homebrewrecovery 拥有 Homebrew tap 的单调 deploy 判定：target Formula
// 只允许前进。它把“请求版本是否落后于 tap 当前版本”变成一个可测试的纯函数，
// 使同一 release_run_id 的重复恢复幂等，并让较旧的恢复请求在写入前 fail closed。
package homebrewrecovery

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"
	"regexp"

	"github.com/FlanChanXwO/pixiv-cli/internal/releaseversion"
)

// Action 是 deploy 判定结果：写入 target 或识别为已发布而 no-op。
type Action string

const (
	// ActionInstall 表示 requested Formula 领先于 tap 当前内容，应当提交。
	ActionInstall Action = "install"
	// ActionNoop 表示 requested Formula 与 tap 当前内容字节完全一致，无需 commit/push。
	ActionNoop Action = "noop"
)

// Decision 描述一次 deploy 判定；Version 来自 requested Formula，CurrentVersion
// 来自 tap 当前 Formula（首次发布时为空）。
type Decision struct {
	Action         Action
	Version        string
	CurrentVersion string
}

// versionDeclaration 匹配 formula 的 `version "x.y.z"` 声明行。
var versionDeclaration = regexp.MustCompile(`(?m)^[ \t]*version[ \t]+"([^"]+)"[ \t]*$`)

// Decide 比较 tap 当前 Formula 与请求写入的 Formula，并按单调语义返回判定：
//
//	requested < current              → 失败（不能回退已发布版本）
//	requested == current 且字节一致  → no-op 成功
//	requested == current 但字节不同  → 失败（同版本不允许出现第二份 Formula）
//	requested > current              → install
//
// current 为空表示 target 尚未发布，此时允许首次写入。
func Decide(current, requested []byte) (Decision, error) {
	requestedVersion, err := parseDeclaredVersion(requested, "requested")
	if err != nil {
		return Decision{}, err
	}
	decision := Decision{Action: ActionInstall, Version: requestedVersion}
	if len(bytes.TrimSpace(current)) == 0 {
		return decision, nil
	}
	currentVersion, err := parseDeclaredVersion(current, "current")
	if err != nil {
		return Decision{}, err
	}
	decision.CurrentVersion = currentVersion

	order, err := releaseversion.Compare(requestedVersion, currentVersion)
	if err != nil {
		return Decision{}, fmt.Errorf("compare requested %q with current %q: %w", requestedVersion, currentVersion, err)
	}
	switch {
	case order < 0:
		return Decision{}, fmt.Errorf("requested version %s is older than the published version %s; refusing to roll the tap back", requestedVersion, currentVersion)
	case order > 0:
		return decision, nil
	case bytes.Equal(current, requested):
		decision.Action = ActionNoop
		return decision, nil
	default:
		return Decision{}, fmt.Errorf("version %s is already published with different content; refusing to republish another formula", requestedVersion)
	}
}

// parseDeclaredVersion 读取 formula 的 version 声明；同一文件出现多个版本声明
// 或缺少声明都视为不可信输入。
func parseDeclaredVersion(formula []byte, label string) (string, error) {
	matches := versionDeclaration.FindAllSubmatch(formula, -1)
	if len(matches) == 0 {
		return "", fmt.Errorf("%s formula declares no version", label)
	}
	if len(matches) > 1 {
		return "", fmt.Errorf("%s formula declares multiple versions", label)
	}
	version := string(matches[0][1])
	if err := releaseversion.Validate(version); err != nil {
		return "", fmt.Errorf("%s formula version: %w", label, err)
	}
	return version, nil
}

// Run 是 scripts/cmd/homebrewrecovery 的入口 owner。调用方传入 tap 当前的 Formula
// 路径（不存在时用空串）与 staging Formula 路径，工具输出 GITHUB_OUTPUT 供 workflow
// 决定是否 commit/push。
func Run(args []string) error {
	flags := flag.NewFlagSet("decide", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	currentPath := flags.String("current", "", "path to the Formula currently published in the tap")
	requestedPath := flags.String("requested", "", "path to the staging Formula about to be published")
	githubOutput := flags.String("github-output", "", "GitHub Actions output file")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("decide accepts no positional arguments: %q", flags.Arg(0))
	}
	if *currentPath == "" {
		return errors.New("--current is required (pass an empty existing path only when the formula is absent)")
	}
	// 决策必须被发布；省略 --github-output 会让 install 判定静默丢失，使 workflow
	// 在报告成功的同时跳过 tap 写入，因此这里要求显式输出路径。
	if *githubOutput == "" {
		return errors.New("--github-output is required so the deploy decision reaches the workflow")
	}
	requested, err := os.ReadFile(*requestedPath)
	if err != nil {
		return fmt.Errorf("read requested formula %q: %w", *requestedPath, err)
	}
	var current []byte
	current, err = os.ReadFile(*currentPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read current formula %q: %w", *currentPath, err)
	}
	decision, err := Decide(current, requested)
	if err != nil {
		return err
	}
	output := fmt.Sprintf("action=%s\nversion=%s\ncurrent_version=%s\n", decision.Action, decision.Version, decision.CurrentVersion)
	if _, err := fmt.Print(output); err != nil {
		return err
	}
	file, err := os.OpenFile(*githubOutput, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = fmt.Fprint(file, output)
	return err
}
