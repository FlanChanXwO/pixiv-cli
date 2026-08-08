package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

const testEvidenceCommit = "0123456789abcdef0123456789abcdef01234567"

func TestMain(m *testing.M) {
	if len(os.Args) == 3 && os.Args[1] == "version" && os.Args[2] == "--json" {
		_, _ = os.Stdout.WriteString(`{"version":"v0.1.0-native-evidence.test","commit":"fixture-commit","build_date":"2026-07-12T00:00:00Z"}` + "\n")
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// TestPolicyCommandHasNoCGODownloadDependency 锁住 runner 的最早 policy gate：它必须能在
// 目标 staticlib 尚未生成时运行，不能因为命令包导入 cgo encoder 而提前链接失败。
func TestPolicyCommandHasNoCGODownloadDependency(t *testing.T) {
	t.Parallel()

	command := exec.Command("go", "list", "-deps", "-f", "{{.ImportPath}}", "./scripts/nativeevidence")
	command.Dir = findRepositoryRoot(t)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("list native evidence command dependencies: %v\n%s", err, output)
	}
	if strings.Contains(string(output), "github.com/FlanChanXwO/pixiv-cli/internal/downloader\n") {
		t.Fatalf("native evidence policy command depends on cgo download package before staticlib generation:\n%s", output)
	}
}
