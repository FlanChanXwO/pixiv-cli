package prverificationworkflow_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// 执行真实 workflow shell，防止可信 base 的旧参数破坏已移除 Rust 的 PR head。
func TestSmokeWorkersBuildBothEncoderSources(t *testing.T) {
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
					Steps []struct{ Name, ID, If, Run string }
				}
			}
			if err := yaml.Unmarshal(body, &document); err != nil {
				t.Fatal(err)
			}
			steps := make(map[string]string)
			for _, step := range document.Jobs["worker"].Steps {
				steps[step.Name] = step.Run
				if strings.HasPrefix(step.Run, "rustup ") || step.Run == "sh scripts/test-rust-vendor.sh" {
					if step.If != "${{ steps.encoder.outputs.rust == 'true' }}" {
						t.Errorf("Rust-only step %q must require a Rust checkout", step.Name)
					}
				}
			}
			for _, rust := range []bool{false, true} {
				t.Run(fmt.Sprintf("rust=%t", rust), func(t *testing.T) {
					dir := t.TempDir()
					if err := os.MkdirAll(filepath.Join(dir, "scripts"), 0o700); err != nil {
						t.Fatal(err)
					}
					if rust {
						manifest := filepath.Join(dir, "internal/media/ugoira/rust/Cargo.toml")
						if err := os.MkdirAll(filepath.Dir(manifest), 0o700); err != nil {
							t.Fatal(err)
						}
						if err := os.WriteFile(manifest, nil, 0o600); err != nil {
							t.Fatal(err)
						}
					}
					buildCall := filepath.Join(dir, "build-call")
					stub := "printf '%s\\n' \"$CGO_ENABLED\" \"$@\" >\"$BUILD_CALL\"\nprintf '%s\\n' \"$BUILD_OUTPUT\"\nexit \"$BUILD_EXIT\"\n"
					if err := os.WriteFile(filepath.Join(dir, "scripts/build-platform.sh"), []byte(stub), 0o700); err != nil {
						t.Fatal(err)
					}
					envPath := filepath.Join(dir, "env")
					if err := os.WriteFile(envPath, nil, 0o600); err != nil {
						t.Fatal(err)
					}
					outputs := filepath.Join(dir, "outputs")
					cgo := "0"
					if rust {
						cgo = "1"
					}
					replace := strings.NewReplacer(
						"${{ matrix.goos }}", "linux", "${{ matrix.goarch }}", "amd64",
						"${{ matrix.rust_target }}", "x86_64-unknown-linux-gnu",
						"${{ matrix.cc }}", "clang -fuse-ld=lld",
						"${{ steps.encoder.outputs.rust }}", fmt.Sprint(rust),
					)
					run := func(script string, exit string) ([]byte, error) {
						cmd := exec.Command("bash", "--noprofile", "--norc", "-eo", "pipefail", "-c", replace.Replace(script))
						cmd.Dir = dir
						cmd.Env = append(os.Environ(), "CGO_ENABLED=1", "GITHUB_ENV="+envPath, "GITHUB_OUTPUT="+outputs,
							"GITHUB_RUN_ID=123", "BUILD_CALL="+buildCall, "BUILD_OUTPUT="+worker.output, "BUILD_EXIT="+exit)
						return cmd.CombinedOutput()
					}
					// GitHub 把前一步 GITHUB_ENV 应用于后一步；source 重放该边界。
					script := steps["Select the checked-out encoder build"] + "\nset -a\nsource \"$GITHUB_ENV\"\nset +a\n" + steps[worker.build]
					if out, err := run(script, "0"); err != nil {
						t.Fatalf("build: %v\n%s", err, out)
					}
					call, err := os.ReadFile(buildCall)
					if err != nil {
						t.Fatal(err)
					}
					want := cgo + "\n--target\nlinux/amd64\n"
					if rust {
						want += "--rust-target\nx86_64-unknown-linux-gnu\n--cc\nclang -fuse-ld=lld\n"
					}
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
					selection, err := os.ReadFile(outputs)
					if err != nil || string(selection) != fmt.Sprintf("rust=%t\n", rust) {
						t.Fatalf("encoder selection: %q, %v", selection, err)
					}
					if out, err := run(script, "23"); err == nil {
						t.Fatalf("build failure discarded: %s", out)
					}
					if worker.file == "platform-smoke.yml" {
						// 仅替换 go 进程边界，执行 workflow 原文的分支和日志处理。
						smoke := "go() { printf '%s\\n' \"$CGO_ENABLED\" \"$@\" >\"$BUILD_CALL\"; return \"$BUILD_EXIT\"; }\n" +
							"export RUNNER_TEMP=\"$PWD\"\nset -a\nsource \"$GITHUB_ENV\"\nset +a\n" + steps["Run native GIF/APNG smoke"]
						if out, err := run(smoke, "0"); err != nil {
							t.Fatalf("encoder smoke: %v\n%s", err, out)
						}
						call, err := os.ReadFile(buildCall)
						if err != nil {
							t.Fatal(err)
						}
						want := cgo + "\ntest\n./internal/media/ugoira/...\n-count=1\n"
						if rust {
							want = cgo + "\ntest\n./internal/media/ugoira\n-run\n^TestRustUgoiraEncoderNativeGIFAndAPNG$\n-count=1\n"
						}
						if string(call) != want {
							t.Errorf("encoder smoke: got %q, want %q", call, want)
						}
						if out, err := run(smoke, "23"); err == nil || !strings.Contains(string(out), "::error title=Native GIF/APNG smoke::") {
							t.Fatalf("smoke failure lost: %v\n%s", err, out)
						}
					}
				})
			}
		})
	}
}
