package citest_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// 执行真实 workflow shell，核对纯 Go 参数及失败传播。
func TestSmokeWorkersBuildGoSources(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture uses POSIX executable permissions")
	}
	for _, worker := range []struct{ file, build, output string }{
		{"platform-smoke.yml", "Build and package the platform binary", "dist/pixiv.tar.gz"},
		{"container-smoke.yml", "Build the versioned Linux binary", "dist/pixiv"},
	} {
		t.Run(worker.file, func(t *testing.T) {
			body, err := os.ReadFile(filepath.Join(repositoryRoot(t), ".github/workflows", worker.file))
			if err != nil {
				t.Fatal(err)
			}
			var document struct {
				Jobs map[string]struct {
					Env   map[string]string
					Steps []struct{ Name, Run string }
				}
			}
			if err := yaml.Unmarshal(body, &document); err != nil {
				t.Fatal(err)
			}
			steps := make(map[string]string)
			for _, step := range document.Jobs["worker"].Steps {
				steps[step.Name] = step.Run
			}
			t.Run("pure-go", func(t *testing.T) {
				dir := t.TempDir()
				if err := os.MkdirAll(filepath.Join(dir, "scripts"), 0o700); err != nil {
					t.Fatal(err)
				}
				buildCall := filepath.Join(dir, "build-call")
				stub := "printf '%s\\n' \"$CGO_ENABLED\" \"$@\" >\"$BUILD_CALL\"\nprintf '%s\\n' \"$BUILD_OUTPUT\"\nexit \"$BUILD_EXIT\"\n"
				if err := os.WriteFile(filepath.Join(dir, "scripts/build-platform.sh"), []byte(stub), 0o700); err != nil {
					t.Fatal(err)
				}
				replace := strings.NewReplacer(
					"${{ matrix.goos }}", "linux", "${{ matrix.goarch }}", "amd64",
				)
				run := func(script string, exit string) ([]byte, error) {
					cmd := exec.Command("bash", "--noprofile", "--norc", "-eo", "pipefail", "-c", replace.Replace(script))
					cmd.Dir = dir
					cmd.Env = append(os.Environ(), "CGO_ENABLED="+document.Jobs["worker"].Env["CGO_ENABLED"], "GITHUB_ENV="+filepath.Join(dir, "env"),
						"GITHUB_RUN_ID=123", "BUILD_CALL="+buildCall, "BUILD_OUTPUT="+worker.output, "BUILD_EXIT="+exit)
					return cmd.CombinedOutput()
				}
				script := steps[worker.build]
				if out, err := run(script, "0"); err != nil {
					t.Fatalf("build: %v\n%s", err, out)
				}
				call, err := os.ReadFile(buildCall)
				if err != nil {
					t.Fatal(err)
				}
				want := "0\n--target\nlinux/amd64\n"
				version := "0.0.0-smoke"
				if worker.file == "platform-smoke.yml" {
					version = "0.2.0-ci.123"
				}
				want += "--version\n" + version + "\n--output-dir\ndist\n"
				if worker.file == "platform-smoke.yml" {
					want += "--package\n"
				}
				if string(call) != want {
					t.Errorf("build contract:\ngot %q\nwant %q", call, want)
				}
				if out, err := run(script, "23"); err == nil {
					t.Fatalf("build failure discarded: %s", out)
				}
				if worker.file == "platform-smoke.yml" {
					// 仅替换 go 进程边界，执行 workflow 原文的分支和日志处理。
					smoke := "go() { printf '%s\\n' \"$CGO_ENABLED\" \"$@\" >\"$BUILD_CALL\"; return \"$BUILD_EXIT\"; }\n" +
						"export RUNNER_TEMP=\"$PWD\"\n" + steps["Run native GIF/APNG smoke"]
					if out, err := run(smoke, "0"); err != nil {
						t.Fatalf("encoder smoke: %v\n%s", err, out)
					}
					call, err := os.ReadFile(buildCall)
					if err != nil {
						t.Fatal(err)
					}
					want := "0\ntest\n./internal/media/ugoira/...\n-count=1\n"
					if string(call) != want {
						t.Errorf("encoder smoke: got %q, want %q", call, want)
					}
					if out, err := run(smoke, "23"); err == nil || !strings.Contains(string(out), "::error title=Native GIF/APNG smoke::") {
						t.Fatalf("smoke failure lost: %v\n%s", err, out)
					}
				}
			})
		})
	}
}
