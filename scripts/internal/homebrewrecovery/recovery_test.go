package homebrewrecovery_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/scripts/internal/homebrewrecovery"
)

// formula 渲染一份最小但结构真实的 Formula，只保留 decision 依赖的 version。
func formula(formulaClass, version string) []byte {
	return []byte(fmt.Sprintf(`class %s < Formula
  desc "Pixiv command-line client and MCP server"
  homepage "https://github.com/FlanChanXwO/pixiv-cli"
  version %q
  license "MIT"

  on_macos do
    on_arm do
      url "https://github.com/FlanChanXwO/pixiv-cli/releases/download/v%s/pixiv-cli_%s_darwin_arm64.tar.gz"
    end
  end
end
`, formulaClass, version, version, version))
}

// Decide 是 Homebrew deploy 的单调状态机：相同版本必须幂等成功且不产生 commit，
// 旧版本必须 fail closed，新版本才允许写入。
func TestDecideSameVersionIdenticalIsIdempotent(t *testing.T) {
	t.Parallel()

	current := formula("PixivCli", "1.1.1")
	decision, err := homebrewrecovery.Decide(current, append([]byte{}, current...))
	if err != nil {
		t.Fatalf("Decide(same version, identical bytes) error = %v", err)
	}
	if decision.Action != homebrewrecovery.ActionNoop {
		t.Fatalf("action = %q, want %q", decision.Action, homebrewrecovery.ActionNoop)
	}
	if decision.Version != "1.1.1" || decision.CurrentVersion != "1.1.1" {
		t.Fatalf("versions = %q / %q, want 1.1.1 / 1.1.1", decision.Version, decision.CurrentVersion)
	}
}

func TestDecideSameVersionDifferentBytesFailsClosed(t *testing.T) {
	t.Parallel()

	// 同一版本不允许出现第二份 Formula：即使版本号相同，只要 bytes 不同就必须
	// 失败，否则 recovery 会静默替换已发布的 digest。
	current := formula("PixivCli", "1.1.1")
	requested := []byte(strings.Replace(string(current), "pixiv-cli_1.1.1", "pixiv-cli_9.9.9", 1))
	if _, err := homebrewrecovery.Decide(current, requested); err == nil {
		t.Fatal("same version with different bytes must fail closed")
	}
}

func TestDecideRejectsOlderRequestedVersion(t *testing.T) {
	t.Parallel()

	current := formula("PixivCli", "1.1.1")
	if _, err := homebrewrecovery.Decide(current, formula("PixivCli", "1.1.0")); err == nil {
		t.Fatal("requested 1.1.0 over current 1.1.1 must fail closed")
	}
	if _, err := homebrewrecovery.Decide(current, formula("PixivCli", "1.0.0")); err == nil {
		t.Fatal("requested 1.0.0 over current 1.1.1 must fail closed")
	}
	// prerelease 不能回退稳定版本（1.2.0-rc.1 < 1.1.1 是 false，但 1.1.1-rc.1 < 1.1.1 是 true）。
	if _, err := homebrewrecovery.Decide(current, formula("PixivCliBeta", "1.1.1-rc.1")); err == nil {
		t.Fatal("requested 1.1.1-rc.1 over current 1.1.1 must fail closed")
	}
}

func TestDecideAcceptsNewerRequestedVersion(t *testing.T) {
	t.Parallel()

	for _, requested := range []string{"1.1.2", "1.2.0", "2.0.0"} {
		decision, err := homebrewrecovery.Decide(formula("PixivCli", "1.1.1"), formula("PixivCli", requested))
		if err != nil {
			t.Fatalf("Decide(1.1.1, %s) error = %v", requested, err)
		}
		if decision.Action != homebrewrecovery.ActionInstall {
			t.Fatalf("Decide(1.1.1, %s) action = %q, want %q", requested, decision.Action, homebrewrecovery.ActionInstall)
		}
	}
}

// 首次发布没有 current Formula：这里必须允许写入，否则第一个 release 无法部署。
func TestDecideAcceptsFirstPublication(t *testing.T) {
	t.Parallel()

	for name, current := range map[string][]byte{
		"nil":   nil,
		"empty": {},
	} {
		decision, err := homebrewrecovery.Decide(current, formula("PixivCli", "1.0.0"))
		if err != nil {
			t.Fatalf("Decide(%s current) error = %v", name, err)
		}
		if decision.Action != homebrewrecovery.ActionInstall {
			t.Fatalf("Decide(%s current) action = %q, want %q", name, decision.Action, homebrewrecovery.ActionInstall)
		}
		if decision.CurrentVersion != "" {
			t.Fatalf("Decide(%s current) current version = %q, want empty", name, decision.CurrentVersion)
		}
	}
}

// 两个 channel 各自独立：stable 的当前版本不能影响 beta 的判断，反之亦然。
func TestDecideChannelsAreIndependent(t *testing.T) {
	t.Parallel()

	stableCurrent := formula("PixivCli", "1.1.1")
	betaCurrent := formula("PixivCliBeta", "2.0.0-rc.1")

	decision, err := homebrewrecovery.Decide(betaCurrent, formula("PixivCliBeta", "2.0.0-rc.2"))
	if err != nil {
		t.Fatalf("beta 2.0.0-rc.2 over beta 2.0.0-rc.1: %v", err)
	}
	if decision.Action != homebrewrecovery.ActionInstall {
		t.Fatalf("beta action = %q, want %q", decision.Action, homebrewrecovery.ActionInstall)
	}

	// beta 的 2.0.0-rc.1 不影响 stable：stable 仍可从 1.1.1 前进到 1.1.2。
	decision, err = homebrewrecovery.Decide(stableCurrent, formula("PixivCli", "1.1.2"))
	if err != nil {
		t.Fatalf("stable 1.1.2 over stable 1.1.1: %v", err)
	}
	if decision.Action != homebrewrecovery.ActionInstall {
		t.Fatalf("stable action = %q, want %q", decision.Action, homebrewrecovery.ActionInstall)
	}
}

// 无法解析 current 版本的 Formula 不能被当作“可以覆盖”：缺少依据时 fail closed。
func TestDecideFailsClosedOnUnparseableCurrentFormula(t *testing.T) {
	t.Parallel()

	broken := []byte("class PixivCli < Formula\n  license \"MIT\"\nend\n")
	if _, err := homebrewrecovery.Decide(broken, formula("PixivCli", "1.1.2")); err == nil {
		t.Fatal("current formula without a version must fail closed")
	}
	if _, err := homebrewrecovery.Decide(broken, []byte("class PixivCli < Formula\nend\n")); err == nil {
		t.Fatal("requested formula without a version must fail closed")
	}
}

// Run 是 workflow 依赖的 CLI 边界：它必须把判定写入 GITHUB_OUTPUT，且当
// tap 中不存在该 Formula 时接受空 current。缺失 current 文件按首次发布处理。
func TestRunEmitsDecisionAndAcceptsMissingCurrentFormula(t *testing.T) {
	dir := t.TempDir()
	requested := filepath.Join(dir, "staging.rb")
	if err := os.WriteFile(requested, formula("PixivCli", "1.1.1"), 0o600); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(dir, "github-output")
	missingCurrent := filepath.Join(dir, "absent.rb")

	if err := homebrewrecovery.Run([]string{
		"--current", missingCurrent,
		"--requested", requested,
		"--github-output", output,
	}); err != nil {
		t.Fatalf("Run(first publication) error = %v", err)
	}
	body, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "action=install") {
		t.Fatalf("first publication must install, got %q", string(body))
	}

	// 同一字节再次执行必须 no-op，并且不返回错误——这正是 R10 的幂等验收。
	current := filepath.Join(dir, "current.rb")
	if err := os.WriteFile(current, formula("PixivCli", "1.1.1"), 0o600); err != nil {
		t.Fatal(err)
	}
	output = filepath.Join(dir, "github-output-2")
	if err := homebrewrecovery.Run([]string{
		"--current", current,
		"--requested", requested,
		"--github-output", output,
	}); err != nil {
		t.Fatalf("Run(same version) error = %v", err)
	}
	body, err = os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "action=noop") {
		t.Fatalf("identical same version must no-op, got %q", string(body))
	}

	// 旧版本必须在写入前失败，而不是返回 install。
	if err := os.WriteFile(requested, formula("PixivCli", "1.1.0"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := homebrewrecovery.Run([]string{
		"--current", current,
		"--requested", requested,
		"--github-output", filepath.Join(dir, "github-output-3"),
	}); err == nil {
		t.Fatal("Run(older version) error = nil, want fail closed")
	}
}

// 决策必须被发布给调用方：若允许省略 --github-output，install 判定仍会 exit 0，
// 但 workflow 的 `action == 'install'` 门会因 output 为空而跳过 tap 写入，同时
// 报告成功——这正是 fail-open。要求输出路径可把“决策丢失”变成硬失败。
func TestRunRequiresGitHubOutputSoDecisionsCannotBeDropped(t *testing.T) {
	dir := t.TempDir()
	requested := filepath.Join(dir, "staging.rb")
	if err := os.WriteFile(requested, formula("PixivCli", "1.1.2"), 0o600); err != nil {
		t.Fatal(err)
	}
	current := filepath.Join(dir, "current.rb")
	if err := os.WriteFile(current, formula("PixivCli", "1.1.1"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := homebrewrecovery.Run([]string{
		"--current", current,
		"--requested", requested,
	}); err == nil {
		t.Fatal("Run without --github-output must fail instead of dropping the install decision")
	}
}
