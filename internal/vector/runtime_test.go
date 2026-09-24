package vector_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/vector"
)

// writePython 写一个回放固定首行的假 runtime 脚本，验证 startup error 映射。
func writePython(t *testing.T, firstLine string) string {
	t.Helper()
	script := filepath.Join(t.TempDir(), "fake-python")
	body := "#!/bin/sh\nprintf '%s\\n' '" + firstLine + "'\n"
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	return script
}

// TestStartSigLIP2MapsStartupErrors 锁定 Workstream F：startup 阶段输出有限的
// 结构化错误类型，Go 层映射成可诊断信息且不泄露 traceback。
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
			t.Setenv("PIXIV_VECTOR_PYTHON", writePython(t, tc.firstLine))
			_, err := vector.StartSigLIP2(context.Background())
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want it to mention %q", err, tc.want)
			}
		})
	}
}

// TestStartSigLIP2MissingInterpreter 报告解释器缺失为 runtime unavailable。
func TestStartSigLIP2MissingInterpreter(t *testing.T) {
	t.Setenv("PIXIV_VECTOR_PYTHON", filepath.Join(t.TempDir(), "missing-python"))
	if _, err := vector.StartSigLIP2(context.Background()); err == nil || !strings.Contains(err.Error(), "embedding runtime unavailable") {
		t.Fatalf("err = %v, want runtime unavailable", err)
	}
}
