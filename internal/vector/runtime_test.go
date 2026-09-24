package vector_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/vector"
)

// TestHelperProcess 不是真正的测试：当 GO_WANT_VECTOR_HELPER=1 时，当前 test
// binary 充当假 Python 解释器，打印 VECTOR_HELPER_OUTPUT 后退出。这是 Go 标准
// helper-process 模式，使 startup error 测试不依赖 Unix shebang 或 .cmd 脚本。
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_VECTOR_HELPER") != "1" {
		return
	}
	fmt.Println(os.Getenv("VECTOR_HELPER_OUTPUT"))
	os.Exit(0)
}

// helperPython 返回一个指向当前 test binary 的假解释器：它会向 stdout 输出
// firstLine 后立即退出，模拟真实 Python 的 startup error 输出。
func helperPython(t *testing.T, firstLine string) func(context.Context) *exec.Cmd {
	t.Helper()
	return func(ctx context.Context) *exec.Cmd {
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=TestHelperProcess", "-")
		cmd.Env = append(os.Environ(),
			"GO_WANT_VECTOR_HELPER=1",
			"VECTOR_HELPER_OUTPUT="+firstLine,
		)
		return cmd
	}
}

// TestStartSigLIP2MapsStartupErrors 锁定 Workstream F：startup 阶段输出有限的
// 结构化错误类型，Go 层映射成可诊断信息且不泄露 traceback。helper-process 模式
// 保证该测试在 Windows 上同样可执行。
func TestStartSigLIP2MapsStartupErrors(t *testing.T) {
	for _, tc := range []struct {
		firstLine string
		want      string
	}{
		{`{"startup_error":"dependency_missing","detail":"torch"}`, "dependency torch is missing"},
		{`{"startup_error":"dependency_version_mismatch","detail":"torch=1.0"}`, "dependency version mismatch"},
		{`{"startup_error":"model_not_found","detail":"preload"}`, "model not found locally"},
		{`{"startup_error":"model_load_failed","detail":"RuntimeError"}`, "model failed to load"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			_, err := vector.StartSigLIP2WithCommand(context.Background(), helperPython(t, tc.firstLine))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want it to mention %q", err, tc.want)
			}
		})
	}
}

// TestStartSigLIP2MissingInterpreter 报告解释器缺失为 runtime unavailable。
func TestStartSigLIP2MissingInterpreter(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing-python")
	_, err := vector.StartSigLIP2WithCommand(context.Background(), func(ctx context.Context) *exec.Cmd {
		return exec.CommandContext(ctx, missing, "-I", "-u", "-c", "pass")
	})
	if err == nil || !strings.Contains(err.Error(), "embedding runtime unavailable") {
		t.Fatalf("err = %v, want runtime unavailable", err)
	}
}
