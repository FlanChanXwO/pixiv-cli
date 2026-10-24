package main

import (
	"fmt"
	"testing"
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
	decision, err := decide(current, append([]byte{}, current...))
	if err != nil {
		t.Fatalf("decide(same version, identical bytes) error = %v", err)
	}
	if decision.action != actionNoop {
		t.Fatalf("action = %q, want %q", decision.action, actionNoop)
	}
	if decision.Version != "1.1.1" || decision.CurrentVersion != "1.1.1" {
		t.Fatalf("versions = %q / %q, want 1.1.1 / 1.1.1", decision.Version, decision.CurrentVersion)
	}
}

func TestDecideRejectsOlderRequestedVersion(t *testing.T) {
	t.Parallel()

	current := formula("PixivCli", "1.1.1")
	if _, err := decide(current, formula("PixivCli", "1.1.0")); err == nil {
		t.Fatal("requested 1.1.0 over current 1.1.1 must fail closed")
	}
	if _, err := decide(current, formula("PixivCli", "1.0.0")); err == nil {
		t.Fatal("requested 1.0.0 over current 1.1.1 must fail closed")
	}
	// prerelease 不能回退稳定版本（1.2.0-rc.1 < 1.1.1 是 false，但 1.1.1-rc.1 < 1.1.1 是 true）。
	if _, err := decide(current, formula("PixivCliBeta", "1.1.1-rc.1")); err == nil {
		t.Fatal("requested 1.1.1-rc.1 over current 1.1.1 must fail closed")
	}
}

func TestDecideFailsClosedOnUnparseableCurrentFormula(t *testing.T) {
	t.Parallel()

	broken := []byte("class PixivCli < Formula\n  license \"MIT\"\nend\n")
	if _, err := decide(broken, formula("PixivCli", "1.1.2")); err == nil {
		t.Fatal("current formula without a version must fail closed")
	}
	if _, err := decide(broken, []byte("class PixivCli < Formula\nend\n")); err == nil {
		t.Fatal("requested formula without a version must fail closed")
	}
}

// Run 是 workflow 依赖的 CLI 边界：它必须把判定写入 GITHUB_OUTPUT，且当
// tap 中不存在该 Formula 时接受空 current。缺失 current 文件按首次发布处理。
