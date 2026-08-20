package main_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestPolicyCommandHasNoCGODownloadDependency 锁住 runner 的最早 policy gate：它必须能在
// 目标 staticlib 尚未生成时运行，不能因为命令包导入 cgo encoder 而提前链接失败。
func TestPolicyCommandHasNoCGODownloadDependency(t *testing.T) {
	t.Parallel()

	command := exec.Command("go", "list", "-deps", "-f", "{{.ImportPath}}", "./scripts/cmd/nativeevidence")
	command.Dir = findNativeEvidenceRepositoryRoot(t)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("list native evidence command dependencies: %v\n%s", err, output)
	}
	if strings.Contains(string(output), "github.com/FlanChanXwO/pixiv-cli/internal/media/downloader\n") {
		t.Fatalf("native evidence policy command depends on cgo download package before staticlib generation:\n%s", output)
	}
}

func findNativeEvidenceRepositoryRoot(t *testing.T) string {
	t.Helper()
	directory, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	for {
		if info, err := os.Stat(filepath.Join(directory, "go.mod")); err == nil && info.Mode().IsRegular() {
			return directory
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			t.Fatal("could not find repository root")
		}
		directory = parent
	}
}
