package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestCheckWorkflowRequiresRustFormatGate(t *testing.T) {
	t.Parallel()

	body, err := os.ReadFile(filepath.Join(findRepositoryRoot(t), ".github", "workflows", "release.yml"))
	if err != nil {
		t.Fatalf("read release workflow: %v", err)
	}
	if err := checkWorkflow(body); err != nil {
		t.Fatalf("release workflow policy rejected checked-in workflow: %v", err)
	}
}

// release staticlib 的字节身份包含 Rust compiler；test gate 与 production rebuild
// 必须按 target 选择生成对应已提交 staticlib 的精确 toolchain。
func TestCheckWorkflowRequiresPinnedRustToolchainForReleaseBuilds(t *testing.T) {
	t.Parallel()

	root := releaseWorkflowRoot(t)
	wantToolchains := map[string]string{
		"x86_64-apple-darwin":       "1.96.0",
		"aarch64-apple-darwin":      "1.96.1",
		"x86_64-unknown-linux-gnu":  "1.96.1",
		"aarch64-unknown-linux-gnu": "1.96.1",
		"x86_64-pc-windows-msvc":    "1.96.0",
		"aarch64-pc-windows-msvc":   "1.96.1",
	}
	for _, test := range []struct {
		job     string
		command string
	}{
		{
			job:     "build",
			command: "rustup toolchain install '${{ matrix.rust_toolchain }}' --profile minimal --component 'clippy,rustfmt' --target '${{ matrix.rust_target }}' --no-self-update",
		},
		{
			job:     "build_production",
			command: "rustup toolchain install '${{ matrix.rust_toolchain }}' --profile minimal --target '${{ matrix.rust_target }}' --no-self-update",
		},
	} {
		t.Run(test.job, func(t *testing.T) {
			job := jobNode(t, root, test.job)
			env := requireMappingValue(t, job, "env")
			toolchain, ok := mappingValue(env, "RUSTUP_TOOLCHAIN")
			if !ok {
				t.Fatalf("%s RUSTUP_TOOLCHAIN is missing", test.job)
			}
			if toolchain.Value != "${{ matrix.rust_toolchain }}" {
				t.Fatalf("%s RUSTUP_TOOLCHAIN = %q, want matrix provenance pin", test.job, toolchain.Value)
			}
			if stepIndexWithRunFragment(requireMappingValue(t, job, "steps").Content, test.command) < 0 {
				t.Fatalf("%s must install the exact pinned Rust toolchain", test.job)
			}

			gotToolchains := make(map[string]string, len(wantToolchains))
			matrix := mustMappingPath(job, "strategy", "matrix")
			include := requireMappingValue(t, matrix, "include")
			for _, entry := range include.Content {
				target := requireMappingValue(t, entry, "rust_target").Value
				gotToolchains[target] = requireMappingValue(t, entry, "rust_toolchain").Value
			}
			if len(gotToolchains) != len(wantToolchains) {
				t.Fatalf("%s pinned Rust target count = %d, want %d", test.job, len(gotToolchains), len(wantToolchains))
			}
			for target, want := range wantToolchains {
				if got := gotToolchains[target]; got != want {
					t.Errorf("%s Rust toolchain for %s = %q, want %q", test.job, target, got, want)
				}
			}
		})
	}
	if err := checkWorkflow(mustMarshalYAML(t, root)); err != nil {
		t.Fatalf("release workflow policy rejected the pinned Rust toolchain: %v", err)
	}
}

func TestCheckWorkflowRejectsReleaseRustToolchainMutations(t *testing.T) {
	t.Parallel()

	matrixEntry := func(t *testing.T, root *yaml.Node, jobName, rustTarget string) *yaml.Node {
		t.Helper()
		matrix := mustMappingPath(jobNode(t, root, jobName), "strategy", "matrix")
		include := requireMappingValue(t, matrix, "include")
		for _, entry := range include.Content {
			if requireMappingValue(t, entry, "rust_target").Value == rustTarget {
				return entry
			}
		}
		t.Fatalf("%s matrix has no Rust target %s", jobName, rustTarget)
		return nil
	}

	for _, test := range []struct {
		name   string
		want   string
		mutate func(t *testing.T, root *yaml.Node)
	}{
		{
			name: "test toolchain env missing",
			want: "build job must bind the audited compiler, Rust toolchain",
			mutate: func(t *testing.T, root *yaml.Node) {
				removeMappingValue(t, requireMappingValue(t, jobNode(t, root, "build"), "env"), "RUSTUP_TOOLCHAIN")
			},
		},
		{
			name: "production toolchain env is mutable stable",
			want: "build_production job must bind CC and the Rust toolchain",
			mutate: func(t *testing.T, root *yaml.Node) {
				requireMappingValue(t, requireMappingValue(t, jobNode(t, root, "build_production"), "env"), "RUSTUP_TOOLCHAIN").Value = "stable"
			},
		},
		{
			name: "test matrix toolchain missing",
			want: "canonical release target fields",
			mutate: func(t *testing.T, root *yaml.Node) {
				entry := matrixEntry(t, root, "build", "x86_64-unknown-linux-gnu")
				removeMappingValue(t, entry, "rust_toolchain")
			},
		},
		{
			name: "test matrix toolchain drifts",
			want: "exactly the six release targets",
			mutate: func(t *testing.T, root *yaml.Node) {
				entry := matrixEntry(t, root, "build", "x86_64-unknown-linux-gnu")
				requireMappingValue(t, entry, "rust_toolchain").Value = "1.97.0"
			},
		},
		{
			name: "production matrix disagrees with test provenance",
			want: "exactly the six release targets",
			mutate: func(t *testing.T, root *yaml.Node) {
				entry := matrixEntry(t, root, "build_production", "x86_64-apple-darwin")
				requireMappingValue(t, entry, "rust_toolchain").Value = "1.96.1"
			},
		},
		{
			name: "test install omits components",
			want: "Install the pinned native Rust toolchain",
			mutate: func(t *testing.T, root *yaml.Node) {
				step := stepWithRun(t, jobNode(t, root, "build"), testRustInstallCommand)
				removeRunFragment(t, step, "--component 'clippy,rustfmt'")
			},
		},
		{
			name: "test install omits target",
			want: "Install the pinned native Rust toolchain",
			mutate: func(t *testing.T, root *yaml.Node) {
				step := stepWithRun(t, jobNode(t, root, "build"), testRustInstallCommand)
				removeRunFragment(t, step, "--target '${{ matrix.rust_target }}'")
			},
		},
		{
			name: "production install omits minimal profile",
			want: "Install the pinned native Rust toolchain must use the required direct command sequence",
			mutate: func(t *testing.T, root *yaml.Node) {
				step := stepWithRun(t, jobNode(t, root, "build_production"), prodRustInstallCommand)
				removeRunFragment(t, step, "--profile minimal")
			},
		},
		{
			name: "production install permits rustup self update",
			want: "Install the pinned native Rust toolchain must use the required direct command sequence",
			mutate: func(t *testing.T, root *yaml.Node) {
				step := stepWithRun(t, jobNode(t, root, "build_production"), prodRustInstallCommand)
				removeRunFragment(t, step, "--no-self-update")
			},
		},
		{
			name: "production install uses stable instead of matrix provenance",
			want: "Install the pinned native Rust toolchain must use the required direct command sequence",
			mutate: func(t *testing.T, root *yaml.Node) {
				step := stepWithRun(t, jobNode(t, root, "build_production"), prodRustInstallCommand)
				replaceRunFragment(t, step, "'${{ matrix.rust_toolchain }}'", "stable")
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := releaseWorkflowRoot(t)
			test.mutate(t, root)
			err := checkWorkflow(mustMarshalYAML(t, root))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("policy error = %v, want rejection containing %q", err, test.want)
			}
		})
	}
}

func TestCheckWorkflowRequiresProductionCacheIsolation(t *testing.T) {
	t.Parallel()

	root := releaseWorkflowRoot(t)
	steps := requireMappingValue(t, jobNode(t, root, "build_production"), "steps")
	with := requireMappingValue(t, steps.Content[1], "with")
	cache, ok := mappingValue(with, "cache")
	if !ok || cache.Value != "false" {
		t.Fatal("build_production setup-go must explicitly disable cross-job Go caches")
	}
}

func TestCheckWorkflowRequiresSixExactProductionArtifactDownloads(t *testing.T) {
	t.Parallel()

	root := releaseWorkflowRoot(t)
	steps := requireMappingValue(t, jobNode(t, root, "publish"), "steps")
	want := []string{
		"verified-release-darwin-amd64",
		"verified-release-darwin-arm64",
		"verified-release-linux-amd64",
		"verified-release-linux-arm64",
		"verified-release-windows-amd64",
		"verified-release-windows-arm64",
	}
	var got []string
	for _, step := range steps.Content {
		uses, ok := mappingValue(step, "uses")
		if !ok || uses.Value != downloadArtifactAction {
			continue
		}
		with := requireMappingValue(t, step, "with")
		got = append(got, requireMappingValue(t, with, "name").Value)
		if requireMappingValue(t, with, "path").Value != "dist" {
			t.Fatal("production artifact download path must be dist")
		}
	}
	if len(got) != len(want) {
		t.Fatalf("production artifact download count = %d, want %d", len(got), len(want))
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("production artifact download %d = %q, want %q", index, got[index], want[index])
		}
	}
}

func TestCheckWorkflowRejectsProductionIsolationMutations(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name   string
		mutate func(t *testing.T, root *yaml.Node)
	}{
		{name: "production job removed", mutate: func(t *testing.T, root *yaml.Node) {
			removeMappingValue(t, requireMappingValue(t, root, "jobs"), "build_production")
		}},
		{name: "test gate uploads verified artifact", mutate: func(t *testing.T, root *yaml.Node) {
			build := jobNode(t, root, "build")
			steps := requireMappingValue(t, build, "steps")
			steps.Content = append(steps.Content, mappingNode("uses", scalarNode(uploadArtifactAction), "with", mappingNode("name", scalarNode("verified-release-${{ matrix.artifact }}"), "path", scalarNode("dist"))))
		}},
		{name: "production bypasses test gate", mutate: func(t *testing.T, root *yaml.Node) {
			requireMappingValue(t, jobNode(t, root, "build_production"), "needs").Value = "validate"
		}},
		{name: "production matrix loses target", mutate: func(t *testing.T, root *yaml.Node) {
			include := requireMappingValue(t, mustMappingPath(jobNode(t, root, "build_production"), "strategy", "matrix"), "include")
			include.Content = include.Content[:len(include.Content)-1]
		}},
		{name: "production matrix gains extra field", mutate: func(t *testing.T, root *yaml.Node) {
			include := requireMappingValue(t, mustMappingPath(jobNode(t, root, "build_production"), "strategy", "matrix"), "include")
			appendMappingValue(t, include.Content[0], "untrusted", scalarNode("value"))
		}},
		{name: "production strategy enables fail fast", mutate: func(t *testing.T, root *yaml.Node) {
			requireMappingValue(t, requireMappingValue(t, jobNode(t, root, "build_production"), "strategy"), "fail-fast").Value = "true"
		}},
		{name: "production job renamed", mutate: func(t *testing.T, root *yaml.Node) {
			requireMappingValue(t, jobNode(t, root, "build_production"), "name").Value = "Untrusted production"
		}},
		{name: "production compiler env changed", mutate: func(t *testing.T, root *yaml.Node) {
			requireMappingValue(t, requireMappingValue(t, jobNode(t, root, "build_production"), "env"), "CC").Value = "cc"
		}},
		{name: "test checkout byte config changed", mutate: func(t *testing.T, root *yaml.Node) {
			requireMappingValue(t, requireMappingValue(t, jobNode(t, root, "build"), "env"), "GIT_CONFIG_VALUE_0").Value = "true"
		}},
		{name: "production adds job env", mutate: func(t *testing.T, root *yaml.Node) {
			appendMappingValue(t, requireMappingValue(t, jobNode(t, root, "build_production"), "env"), "EXTRA", scalarNode("value"))
		}},
		{name: "production elevates token permissions", mutate: func(t *testing.T, root *yaml.Node) {
			requireMappingValue(t, requireMappingValue(t, jobNode(t, root, "build_production"), "permissions"), "contents").Value = "write"
		}},
		{name: "production conditionally skips", mutate: func(t *testing.T, root *yaml.Node) {
			appendMappingValue(t, jobNode(t, root, "build_production"), "if", scalarNode("false"))
		}},
		{name: "production adds environment", mutate: func(t *testing.T, root *yaml.Node) {
			appendMappingValue(t, jobNode(t, root, "build_production"), "environment", scalarNode("release"))
		}},
		{name: "production references secret", mutate: func(t *testing.T, root *yaml.Node) {
			steps := requireMappingValue(t, jobNode(t, root, "build_production"), "steps")
			appendMappingValue(t, steps.Content[0], "env", mappingNode("LEAK", scalarNode("${{ secrets.RELEASE_SIGNING_PRIVATE_KEY }}")))
		}},
		{name: "production runs quality overlay", mutate: func(t *testing.T, root *yaml.Node) {
			insertRunStep(t, jobNode(t, root, "build_production"), 4, "Forbidden quality gate", "go test ./...")
		}},
		{name: "production adds arbitrary step", mutate: func(t *testing.T, root *yaml.Node) {
			insertRunStep(t, jobNode(t, root, "build_production"), 2, "Arbitrary production step", "true")
		}},
		{name: "production checkout uses main", mutate: func(t *testing.T, root *yaml.Node) {
			checkout := checkoutStep(t, jobNode(t, root, "build_production"))
			requireMappingValue(t, requireMappingValue(t, checkout, "with"), "ref").Value = "main"
		}},
		{name: "production checkout adds repository", mutate: func(t *testing.T, root *yaml.Node) {
			checkout := checkoutStep(t, jobNode(t, root, "build_production"))
			appendMappingValue(t, requireMappingValue(t, checkout, "with"), "repository", scalarNode("owner/other"))
		}},
		{name: "production setup Go cache omitted", mutate: func(t *testing.T, root *yaml.Node) {
			steps := requireMappingValue(t, jobNode(t, root, "build_production"), "steps")
			removeMappingValue(t, requireMappingValue(t, steps.Content[1], "with"), "cache")
		}},
		{name: "production setup Go cache enabled", mutate: func(t *testing.T, root *yaml.Node) {
			steps := requireMappingValue(t, jobNode(t, root, "build_production"), "steps")
			requireMappingValue(t, requireMappingValue(t, steps.Content[1], "with"), "cache").Value = "true"
		}},
		{name: "production uploads test artifact", mutate: func(t *testing.T, root *yaml.Node) {
			steps := requireMappingValue(t, jobNode(t, root, "build_production"), "steps")
			upload := steps.Content[len(steps.Content)-1]
			requireMappingValue(t, requireMappingValue(t, upload, "with"), "name").Value = "test-gate-${{ matrix.artifact }}"
		}},
		{name: "verify bypasses production", mutate: func(t *testing.T, root *yaml.Node) {
			requireMappingValue(t, jobNode(t, root, "verify_release_source"), "needs").Value = "build"
		}},
		{name: "publish downloads test artifacts", mutate: func(t *testing.T, root *yaml.Node) {
			steps := requireMappingValue(t, jobNode(t, root, "publish"), "steps")
			download := steps.Content[2]
			requireMappingValue(t, requireMappingValue(t, download, "with"), "name").Value = "test-gate-darwin-amd64"
		}},
		{name: "publish production download missing", mutate: func(t *testing.T, root *yaml.Node) {
			steps := requireMappingValue(t, jobNode(t, root, "publish"), "steps")
			steps.Content = append(steps.Content[:2], steps.Content[3:]...)
		}},
		{name: "publish production download duplicated", mutate: func(t *testing.T, root *yaml.Node) {
			steps := requireMappingValue(t, jobNode(t, root, "publish"), "steps")
			first := requireMappingValue(t, requireMappingValue(t, steps.Content[2], "with"), "name").Value
			requireMappingValue(t, requireMappingValue(t, steps.Content[3], "with"), "name").Value = first
		}},
		{name: "publish production download renamed", mutate: func(t *testing.T, root *yaml.Node) {
			steps := requireMappingValue(t, jobNode(t, root, "publish"), "steps")
			requireMappingValue(t, requireMappingValue(t, steps.Content[2], "with"), "name").Value = "verified-release-other"
		}},
		{name: "publish production download path changed", mutate: func(t *testing.T, root *yaml.Node) {
			steps := requireMappingValue(t, jobNode(t, root, "publish"), "steps")
			requireMappingValue(t, requireMappingValue(t, steps.Content[2], "with"), "path").Value = "other"
		}},
		{name: "publish production download uses pattern", mutate: func(t *testing.T, root *yaml.Node) {
			steps := requireMappingValue(t, jobNode(t, root, "publish"), "steps")
			with := requireMappingValue(t, steps.Content[2], "with")
			removeMappingValue(t, with, "name")
			appendMappingValue(t, with, "pattern", scalarNode("verified-release-*"))
		}},
		{name: "publish adds second artifact download", mutate: func(t *testing.T, root *yaml.Node) {
			publish := jobNode(t, root, "publish")
			steps := requireMappingValue(t, publish, "steps")
			extra := mappingNode(
				"uses", scalarNode(downloadArtifactAction),
				"with", mappingNode(
					"pattern", scalarNode("test-gate-*"),
					"path", scalarNode("dist"),
					"merge-multiple", scalarNode("true"),
				),
			)
			steps.Content = append(steps.Content, nil)
			copy(steps.Content[4:], steps.Content[3:])
			steps.Content[3] = extra
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := releaseWorkflowRoot(t)
			test.mutate(t, root)
			if err := checkWorkflow(mustMarshalYAML(t, root)); err == nil {
				t.Fatal("release workflow policy accepted a production-isolation mutation")
			}
		})
	}
}

func TestCheckWorkflowAcceptsIndependentReleaseSourceVerification(t *testing.T) {
	t.Parallel()

	body, err := os.ReadFile(filepath.Join(findRepositoryRoot(t), ".github", "workflows", "release.yml"))
	if err != nil {
		t.Fatalf("read release workflow: %v", err)
	}
	err = checkWorkflow(body)
	if err != nil {
		t.Fatalf("release workflow policy rejected independent verify_release_source job: %v", err)
	}
}

func TestCheckWorkflowRequiresHomebrewReleaseGate(t *testing.T) {
	t.Parallel()

	root := releaseWorkflowRoot(t)
	for _, name := range []string{"render_homebrew_formula", "verify_homebrew_formula", "deploy_homebrew_tap"} {
		if _, ok := mappingValue(requireMappingValue(t, root, "jobs"), name); !ok {
			t.Errorf("release workflow is missing %q job", name)
		}
	}
	if err := checkWorkflow(mustMarshalYAML(t, root)); err != nil {
		t.Fatalf("release workflow policy rejected Homebrew release gate: %v", err)
	}
}

// Homebrew 6 不接受任意 workspace formula 路径；四平台门禁必须通过隔离 staging tap
// 的 tap-qualified 标识符安装。
func TestCheckWorkflowRequiresTapQualifiedStagingFormulaInstall(t *testing.T) {
	t.Parallel()

	root := releaseWorkflowRoot(t)
	step := stepWithRun(t, jobNode(t, root, "verify_homebrew_formula"), "brew install")
	replaceRunFragment(t, step, "brew install --formula \"$staging_tap/$formula_name\"", "brew install --formula \"staging-formula/$formula_name.rb\"")
	err := checkWorkflow(mustMarshalYAML(t, root))
	if err == nil || !strings.Contains(err.Error(), "Homebrew native install gate must use the required direct command sequence") {
		t.Fatalf("policy error = %v, want workspace-path Homebrew formula install rejection", err)
	}
}

// Homebrew 6 默认要求 tap trust；staging tap 只在 runner 本地临时存在，仍必须通过
// `brew trust --tap` 显式登记，不能用环境变量或 developer mode 绕过。
func TestCheckWorkflowRequiresStagingTapTrust(t *testing.T) {
	t.Parallel()

	root := releaseWorkflowRoot(t)
	step := stepWithRun(t, jobNode(t, root, "verify_homebrew_formula"), "brew trust --tap")
	removeRunFragment(t, step, "brew trust --tap \"$staging_tap\"")
	err := checkWorkflow(mustMarshalYAML(t, root))
	if err == nil || !strings.Contains(err.Error(), "Homebrew native install gate must use the required direct command sequence") {
		t.Fatalf("policy error = %v, want untrusted staging tap install rejection", err)
	}
}

func TestCheckPinnedGitHubKnownHosts(t *testing.T) {
	t.Parallel()

	body, err := os.ReadFile(filepath.Join(findRepositoryRoot(t), "templates", "homebrew", "github.com-known-hosts"))
	if err != nil {
		t.Fatalf("read pinned GitHub known_hosts: %v", err)
	}
	if err := checkPinnedGitHubKnownHosts(body); err != nil {
		t.Fatalf("checked-in GitHub known_hosts rejected: %v", err)
	}
	// 先归一化再构造 CRLF，避免 Windows 已经以 CRLF 检出的 fixture
	// 再替换换行时形成 \r\r\n，确保这里覆盖真实的 Windows 输入。
	crlfBody := []byte(strings.ReplaceAll(strings.ReplaceAll(string(body), "\r\n", "\n"), "\n", "\r\n"))
	if err := checkPinnedGitHubKnownHosts(crlfBody); err != nil {
		t.Fatalf("CRLF checked-out GitHub known_hosts rejected: %v", err)
	}
	for _, mutation := range [][]byte{
		[]byte("github.com ssh-ed25519 attacker\n"),
		append(append([]byte(nil), body...), []byte("github.com ssh-rsa extra\n")...),
		append(append([]byte(nil), crlfBody...), []byte("github.com ssh-rsa extra\r\n")...),
	} {
		if err := checkPinnedGitHubKnownHosts(mutation); err == nil {
			t.Fatal("mutated GitHub known_hosts fixture was accepted")
		}
	}
}

// 不可变 tag 已经存在时，只允许从默认分支受审计 workflow 以精确 tag 恢复；
// 生产版本、checkout 和发布命令必须继续绑定同一个 RELEASE_TAG。
func TestCheckRecoveryPolicyRequiresTrustedReleaseTag(t *testing.T) {
	t.Parallel()

	root := releaseWorkflowRoot(t)
	if err := checkRecoveryPolicy(root); err != nil {
		t.Fatalf("checked-in recovery policy rejected: %v", err)
	}
}

// v0.3.0 tag 已包含顶层 account external test；恢复只覆盖 tag 与默认分支之间
// 实际变化的 workflow 及其 verifier，生产源码仍只来自 tag。
func TestCheckRecoveryPolicyRequiresExactThreePathOverlay(t *testing.T) {
	t.Parallel()

	const commands = `set -euo pipefail
test -z "$(git diff --name-only)"
test -z "$(git diff --cached --name-only)"
git archive --format=tar "$GITHUB_SHA" -- \
  .github/workflows/release.yml \
  scripts/releaseworkflow/main.go \
  scripts/releaseworkflow/main_test.go | tar -xf -
test "$(git diff --name-only)" = "$(printf '%s\n' \
  .github/workflows/release.yml \
  scripts/releaseworkflow/main.go \
  scripts/releaseworkflow/main_test.go)"
test -z "$(git diff --cached --name-only)"`
	paths := []string{
		".github/workflows/release.yml",
		"scripts/releaseworkflow/main.go",
		"scripts/releaseworkflow/main_test.go",
	}
	root := releaseWorkflowRoot(t)
	step := stepWithRun(t, jobNode(t, root, "build"), `git archive --format=tar "$GITHUB_SHA"`)
	run := requireMappingValue(t, step, "run")
	if run.Value != commands+"\n" {
		t.Fatalf("recovery overlay command = %q, want exact three-path audited command", run.Value)
	}
	if err := checkRecoveryPolicy(root); err != nil {
		t.Fatalf("checked-in recovery policy rejected: %v", err)
	}

	for _, path := range paths {
		t.Run("required path omitted: "+path, func(t *testing.T) {
			root := releaseWorkflowRoot(t)
			step := stepWithRun(t, jobNode(t, root, "build"), `git archive --format=tar "$GITHUB_SHA"`)
			run := requireMappingValue(t, step, "run")
			run.Value = strings.Replace(run.Value, path, "", 1)
			if err := checkRecoveryPolicy(root); err == nil {
				t.Fatal("release recovery policy accepted a missing audited overlay path")
			}
		})
	}
	t.Run("account external test re-added", func(t *testing.T) {
		root := releaseWorkflowRoot(t)
		step := stepWithRun(t, jobNode(t, root, "build"), `git archive --format=tar "$GITHUB_SHA"`)
		run := requireMappingValue(t, step, "run")
		run.Value = strings.ReplaceAll(run.Value, "  scripts/releaseworkflow/main.go \\", "  pixiv/account_external_test.go \\\n  scripts/releaseworkflow/main.go \\")
		if err := checkRecoveryPolicy(root); err == nil {
			t.Fatal("release recovery policy accepted the redundant account test overlay")
		}
	})
	t.Run("arbitrary fourth path added", func(t *testing.T) {
		root := releaseWorkflowRoot(t)
		step := stepWithRun(t, jobNode(t, root, "build"), `git archive --format=tar "$GITHUB_SHA"`)
		run := requireMappingValue(t, step, "run")
		run.Value = strings.ReplaceAll(run.Value, "  scripts/releaseworkflow/main.go \\", "  pkg/pixiv/other_test.go \\\n  scripts/releaseworkflow/main.go \\")
		if err := checkRecoveryPolicy(root); err == nil {
			t.Fatal("release recovery policy accepted an extra overlay path")
		}
	})
}

// Windows 的 Go+cgo 质量门和最终 binary 必须统一使用 Clang+LLD，不能只修某个 step。
func TestCheckRecoveryPolicyRequiresWindowsClangLLDForWholeBuildJob(t *testing.T) {
	t.Parallel()

	root := releaseWorkflowRoot(t)
	build := jobNode(t, root, "build")
	matrix := requireMappingValue(t, requireMappingValue(t, build, "strategy"), "matrix")
	include := requireMappingValue(t, matrix, "include")
	for _, entry := range include.Content {
		goos := requireMappingValue(t, entry, "goos").Value
		if goos != "windows" {
			continue
		}
		removeMappingValue(t, entry, "cc")
		break
	}
	if err := checkRecoveryPolicy(root); err == nil || !strings.Contains(err.Error(), "build matrix entries must contain only the canonical release target fields") {
		t.Fatalf("policy error = %v, want Windows Clang+LLD rejection", err)
	}
}

func TestCheckRecoveryPolicyRequiresDispatchFromDefaultBranch(t *testing.T) {
	t.Parallel()

	root := releaseWorkflowRoot(t)
	step := stepWithRun(t, jobNode(t, root, "validate"), `test "$GITHUB_REF" = "refs/heads/$DEFAULT_BRANCH"`)
	removeCommand(t, step, `test "$GITHUB_REF" = "refs/heads/$DEFAULT_BRANCH"`)
	if err := checkWorkflow(mustMarshalYAML(t, root)); err == nil || !strings.Contains(err.Error(), "refs/heads/$DEFAULT_BRANCH") {
		t.Fatalf("policy error = %v, want dispatch default-branch rejection", err)
	}
}

func TestCheckRecoveryPolicyRejectsRecoveryTrustMutations(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name   string
		mutate func(t *testing.T, root *yaml.Node)
	}{
		{name: "dispatch removed", mutate: func(t *testing.T, root *yaml.Node) {
			removeMappingValue(t, requireMappingValue(t, root, "on"), "workflow_dispatch")
		}},
		{name: "dispatch tag optional", mutate: func(t *testing.T, root *yaml.Node) {
			dispatch := requireMappingValue(t, requireMappingValue(t, root, "on"), "workflow_dispatch")
			tag := requireMappingValue(t, requireMappingValue(t, dispatch, "inputs"), "release_tag")
			requireMappingValue(t, tag, "required").Value = "false"
		}},
		{name: "release tag bound to arbitrary sha", mutate: func(t *testing.T, root *yaml.Node) {
			requireMappingValue(t, requireMappingValue(t, root, "env"), "RELEASE_TAG").Value = "${{ inputs.sha }}"
		}},
		{name: "existing release check removed", mutate: func(t *testing.T, root *yaml.Node) {
			step := stepWithRun(t, jobNode(t, root, "validate"), "gh api --include")
			removeRunFragment(t, step, `gh api --include "repos/$GITHUB_REPOSITORY/releases/tags/$RELEASE_TAG"`)
		}},
		{name: "overlay writes production source", mutate: func(t *testing.T, root *yaml.Node) {
			step := stepWithRun(t, jobNode(t, root, "build"), `git archive --format=tar "$GITHUB_SHA"`)
			run := requireMappingValue(t, step, "run")
			run.Value = strings.ReplaceAll(run.Value, "  scripts/releaseworkflow/main.go \\", "  pixiv/account.go \\\n  scripts/releaseworkflow/main.go \\")
		}},
		{name: "production source switches to workflow sha", mutate: func(t *testing.T, root *yaml.Node) {
			appendRunStep(t, jobNode(t, root, "build"), "Bypass immutable tag", `git checkout "$GITHUB_SHA"`)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := releaseWorkflowRoot(t)
			test.mutate(t, root)
			if err := checkWorkflow(mustMarshalYAML(t, root)); err == nil {
				t.Fatal("release recovery policy accepted a trust mutation")
			}
		})
	}
}

func TestCheckWorkflowRejectsTestGateProductionMutations(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name   string
		mutate func(t *testing.T, root *yaml.Node)
	}{
		{name: "production build command inserted", mutate: func(t *testing.T, root *yaml.Node) {
			insertRunStep(t, jobNode(t, root, "build"), 14, "Forbidden production build", "go build -trimpath ./cmd/pixiv")
		}},
		{name: "artifact upload inserted", mutate: func(t *testing.T, root *yaml.Node) {
			build := jobNode(t, root, "build")
			steps := requireMappingValue(t, build, "steps")
			steps.Content = append(steps.Content, mappingNode("uses", scalarNode(uploadArtifactAction), "with", mappingNode("name", scalarNode("test-gate-extra"), "path", scalarNode("dist"))))
		}},
		{name: "race and vet gates reordered", mutate: func(t *testing.T, root *yaml.Node) {
			steps := requireMappingValue(t, jobNode(t, root, "build"), "steps")
			raceIndex := stepIndexWithRunFragment(steps.Content, "go test -race ./...")
			vetIndex := stepIndexWithRunFragment(steps.Content, "go vet ./...")
			steps.Content[raceIndex], steps.Content[vetIndex] = steps.Content[vetIndex], steps.Content[raceIndex]
		}},
		{name: "license gate deleted from overlay sequence", mutate: func(t *testing.T, root *yaml.Node) {
			steps := requireMappingValue(t, jobNode(t, root, "build"), "steps")
			index := stepIndexWithRunFragment(steps.Content, "go run ./scripts/licensebundle --check")
			steps.Content = append(steps.Content[:index], steps.Content[index+1:]...)
		}},
		{name: "quality gate deleted", mutate: func(t *testing.T, root *yaml.Node) {
			steps := requireMappingValue(t, jobNode(t, root, "build"), "steps")
			index := stepIndexWithRunFragment(steps.Content, "go run ./scripts/licensebundle --check")
			steps.Content = append(steps.Content[:index], steps.Content[index+1:]...)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := releaseWorkflowRoot(t)
			test.mutate(t, root)
			if err := checkWorkflow(mustMarshalYAML(t, root)); err == nil {
				t.Fatal("release recovery policy accepted an overlay/reset mutation")
			}
		})
	}
}

func TestCheckWorkflowRejectsHomebrewReleaseGateMutations(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name   string
		want   string
		mutate func(t *testing.T, root *yaml.Node)
	}{
		{
			name: "render bypasses published checksums",
			want: "render_homebrew_formula job: needs must equal \"publish\"",
			mutate: func(t *testing.T, root *yaml.Node) {
				requireMappingValue(t, jobNode(t, root, "render_homebrew_formula"), "needs").Value = "verify_release_source"
			},
		},
		{
			name: "install bypasses rendered formula",
			want: "verify_homebrew_formula job: needs must equal \"render_homebrew_formula\"",
			mutate: func(t *testing.T, root *yaml.Node) {
				requireMappingValue(t, jobNode(t, root, "verify_homebrew_formula"), "needs").Value = "publish"
			},
		},
		{
			name: "tap deploy bypasses install matrix",
			want: "deploy_homebrew_tap job: needs must equal \"verify_homebrew_formula\"",
			mutate: func(t *testing.T, root *yaml.Node) {
				requireMappingValue(t, jobNode(t, root, "deploy_homebrew_tap"), "needs").Value = "render_homebrew_formula"
			},
		},
		{
			name: "publish exports a different checksum file",
			want: "publish job must upload the verified release/checksums.txt after publishing",
			mutate: func(t *testing.T, root *yaml.Node) {
				steps := requireMappingValue(t, jobNode(t, root, "publish"), "steps")
				upload := steps.Content[len(steps.Content)-1]
				requireMappingValue(t, requireMappingValue(t, upload, "with"), "path").Value = "dist/checksums.txt"
			},
		},
		{
			name: "publish mutates checksums after Release verification",
			want: "must preserve the verified asset set before exporting checksums",
			mutate: func(t *testing.T, root *yaml.Node) {
				step := stepWithRun(t, jobNode(t, root, "publish"), "gh release edit")
				replaceRunFragment(t, step, "gh release edit \"$RELEASE_TAG\" --draft=false", "printf tampered > release/checksums.txt\ngh release edit \"$RELEASE_TAG\" --draft=false")
			},
		},
		{
			name: "checksum artifact is not immediately downstream of publication",
			want: "must upload the verified release/checksums.txt after publishing",
			mutate: func(t *testing.T, root *yaml.Node) {
				steps := requireMappingValue(t, jobNode(t, root, "publish"), "steps")
				steps.Content = append(steps.Content, runStepNode("Intervening mutation opportunity", "true"))
			},
		},
		{
			name: "render downloads a different artifact",
			want: "verified checksums download must be the exact pinned action step",
			mutate: func(t *testing.T, root *yaml.Node) {
				step := requireMappingValue(t, jobNode(t, root, "render_homebrew_formula"), "steps").Content[2]
				requireMappingValue(t, requireMappingValue(t, step, "with"), "name").Value = "unverified-checksums"
			},
		},
		{
			name: "stable channel renders beta formula",
			want: "Homebrew formula render step must use the required direct command sequence",
			mutate: func(t *testing.T, root *yaml.Node) {
				replaceRunFragment(t, stepWithRun(t, jobNode(t, root, "render_homebrew_formula"), "formula_name=pixiv-cli"), "formula_name=pixiv-cli\n", "formula_name=pixiv-cli-beta\n")
			},
		},
		{
			name: "formula output artifact renamed",
			want: "staging formula artifact must be the exact pinned action step",
			mutate: func(t *testing.T, root *yaml.Node) {
				steps := requireMappingValue(t, jobNode(t, root, "render_homebrew_formula"), "steps")
				upload := steps.Content[len(steps.Content)-1]
				requireMappingValue(t, requireMappingValue(t, upload, "with"), "name").Value = "mutable-formula"
			},
		},
		{
			name: "native install matrix loses arm64 Linux",
			want: "matrix must contain exactly the four native targets",
			mutate: func(t *testing.T, root *yaml.Node) {
				strategy := requireMappingValue(t, jobNode(t, root, "verify_homebrew_formula"), "strategy")
				include := requireMappingValue(t, requireMappingValue(t, strategy, "matrix"), "include")
				requireMappingValue(t, include.Content[len(include.Content)-1], "runner").Value = "ubuntu-24.04"
			},
		},
		{
			name: "Linux Homebrew activation removed",
			want: "Homebrew native install gate must use the required direct command sequence",
			mutate: func(t *testing.T, root *yaml.Node) {
				removeRunFragment(t, stepWithRun(t, jobNode(t, root, "verify_homebrew_formula"), "/home/linuxbrew/.linuxbrew/bin/brew"), "eval \"$(/home/linuxbrew/.linuxbrew/bin/brew shellenv)\"")
			},
		},
		{
			name: "staging formula install is not tap qualified",
			want: "Homebrew native install gate must use the required direct command sequence",
			mutate: func(t *testing.T, root *yaml.Node) {
				replaceRunFragment(t, stepWithRun(t, jobNode(t, root, "verify_homebrew_formula"), "brew install --formula"), "brew install --formula \"$staging_tap/$formula_name\"", "brew install --formula \"staging-formula/$formula_name.rb\"")
			},
		},
		{
			name: "staging formula is copied into the trusted tap namespace",
			want: "Homebrew native install gate must use the required direct command sequence",
			mutate: func(t *testing.T, root *yaml.Node) {
				replaceRunFragment(t, stepWithRun(t, jobNode(t, root, "verify_homebrew_formula"), "staging_tap="), "staging_tap=pixiv-cli-release/staging", "staging_tap=FlanChanXwO/tap")
			},
		},
		{
			name: "staging tap trust is removed",
			want: "Homebrew native install gate must use the required direct command sequence",
			mutate: func(t *testing.T, root *yaml.Node) {
				removeRunFragment(t, stepWithRun(t, jobNode(t, root, "verify_homebrew_formula"), "brew trust --tap"), "brew trust --tap \"$staging_tap\"")
			},
		},
		{
			name: "staging tap trust is redirected",
			want: "Homebrew native install gate must use the required direct command sequence",
			mutate: func(t *testing.T, root *yaml.Node) {
				replaceRunFragment(t, stepWithRun(t, jobNode(t, root, "verify_homebrew_formula"), "brew trust --tap"), "brew trust --tap \"$staging_tap\"", "brew trust --tap FlanChanXwO/tap")
			},
		},
		{
			name: "version JSON assertion removed",
			want: "Homebrew native install gate must use the required direct command sequence",
			mutate: func(t *testing.T, root *yaml.Node) {
				removeRunFragment(t, stepWithRun(t, jobNode(t, root, "verify_homebrew_formula"), "pixiv version --json"), "pixiv version --json")
			},
		},
		{
			name: "install gate references a secret",
			want: "Homebrew render and install jobs must not reference secrets",
			mutate: func(t *testing.T, root *yaml.Node) {
				appendMappingValue(t, requireMappingValue(t, jobNode(t, root, "verify_homebrew_formula"), "steps").Content[1], "env", mappingNode("LEAK", scalarNode("${{ secrets.HOMEBREW_TAP_DEPLOY_KEY }}")))
			},
		},
		{
			name: "tap deploy is unprotected",
			want: "deploy_homebrew_tap environment must be release",
			mutate: func(t *testing.T, root *yaml.Node) {
				requireMappingValue(t, jobNode(t, root, "deploy_homebrew_tap"), "environment").Value = "unprotected"
			},
		},
		{
			name: "tap deploy runs after a failed install gate",
			want: "deploy_homebrew_tap job must not define if or continue-on-error",
			mutate: func(t *testing.T, root *yaml.Node) {
				appendMappingValue(t, jobNode(t, root, "deploy_homebrew_tap"), "if", scalarNode("always()"))
			},
		},
		{
			name: "tap deploy elevates token permission",
			want: "permissions must contain only contents: read",
			mutate: func(t *testing.T, root *yaml.Node) {
				permissions := requireMappingValue(t, jobNode(t, root, "deploy_homebrew_tap"), "permissions")
				requireMappingValue(t, permissions, "contents").Value = "write"
			},
		},
		{
			name: "tap secret is reachable before final push",
			want: "must not reference secrets outside its final push step",
			mutate: func(t *testing.T, root *yaml.Node) {
				steps := requireMappingValue(t, jobNode(t, root, "deploy_homebrew_tap"), "steps")
				appendMappingValue(t, steps.Content[1], "env", mappingNode("EARLY_KEY", scalarNode("${{ secrets.HOMEBREW_TAP_DEPLOY_KEY }}")))
			},
		},
		{
			name: "tap push adds an unrelated secret",
			want: "tap final push step must declare only HOMEBREW_TAP_DEPLOY_KEY",
			mutate: func(t *testing.T, root *yaml.Node) {
				steps := requireMappingValue(t, jobNode(t, root, "deploy_homebrew_tap"), "steps")
				env := requireMappingValue(t, steps.Content[len(steps.Content)-1], "env")
				appendMappingValue(t, env, "UNRELATED", scalarNode("${{ secrets.UNRELATED }}"))
			},
		},
		{
			name: "tap clone uses SSH before verification",
			want: "tap one-formula commit step must use the required direct command sequence",
			mutate: func(t *testing.T, root *yaml.Node) {
				replaceRunFragment(t, stepWithRun(t, jobNode(t, root, "deploy_homebrew_tap"), "git clone https://"), "https://github.com/FlanChanXwO/homebrew-tap.git", "git@github.com:FlanChanXwO/homebrew-tap.git")
			},
		},
		{
			name: "tap commit stages the whole repository",
			want: "tap one-formula commit step must use the required direct command sequence",
			mutate: func(t *testing.T, root *yaml.Node) {
				replaceRunFragment(t, stepWithRun(t, jobNode(t, root, "deploy_homebrew_tap"), "git -C \"$tap_dir\" add"), "git -C \"$tap_dir\" add -- \"Formula/$formula_name.rb\"", "git -C \"$tap_dir\" add -- .")
			},
		},
		{
			name: "strict host key checking disabled",
			want: "tap final protected push step must use the required direct command sequence",
			mutate: func(t *testing.T, root *yaml.Node) {
				replaceRunFragment(t, stepWithRun(t, jobNode(t, root, "deploy_homebrew_tap"), "StrictHostKeyChecking=yes"), "StrictHostKeyChecking=yes", "StrictHostKeyChecking=no")
			},
		},
		{
			name: "tap private key loses required trailing newline",
			want: "tap final protected push step must use the required direct command sequence",
			mutate: func(t *testing.T, root *yaml.Node) {
				replaceRunFragment(t, stepWithRun(t, jobNode(t, root, "deploy_homebrew_tap"), "HOMEBREW_TAP_DEPLOY_KEY"), "printf '%s\\n' \"$HOMEBREW_TAP_DEPLOY_KEY\"", "printf '%s' \"$HOMEBREW_TAP_DEPLOY_KEY\"")
			},
		},
		{
			name: "tap push changes the exact SSH remote",
			want: "tap final protected push step must use the required direct command sequence",
			mutate: func(t *testing.T, root *yaml.Node) {
				replaceRunFragment(t, stepWithRun(t, jobNode(t, root, "deploy_homebrew_tap"), "remote set-url"), "git@github.com:FlanChanXwO/homebrew-tap.git", "git@github.com:attacker/homebrew-tap.git")
			},
		},
		{
			name: "known hosts replaced by ssh-keyscan",
			want: "tap final protected push step must use the required direct command sequence",
			mutate: func(t *testing.T, root *yaml.Node) {
				replaceRunFragment(t, stepWithRun(t, jobNode(t, root, "deploy_homebrew_tap"), "UserKnownHostsFile="), "UserKnownHostsFile=$GITHUB_WORKSPACE/templates/homebrew/github.com-known-hosts", "UserKnownHostsFile=$RUNNER_TEMP/known_hosts; ssh-keyscan github.com")
			},
		},
		{
			name: "push does not target main from the verified commit",
			want: "tap final protected push step must use the required direct command sequence",
			mutate: func(t *testing.T, root *yaml.Node) {
				replaceRunFragment(t, stepWithRun(t, jobNode(t, root, "deploy_homebrew_tap"), "push origin HEAD:main"), "push origin HEAD:main", "push origin HEAD")
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := releaseWorkflowRoot(t)
			test.mutate(t, root)
			err := checkWorkflow(mustMarshalYAML(t, root))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("policy error = %v, want rejection containing %q", err, test.want)
			}
		})
	}
}

// Task34 将 formula 渲染、四个平台实际安装和 tap 写入串成发布后的强制门禁；删除最终
// deploy job 必须被结构化策略拒绝，避免安装成功后 workflow 静默结束而未更新 tap。
func TestCheckWorkflowRequiresHomebrewTapPublicationGate(t *testing.T) {
	t.Parallel()

	root := releaseWorkflowRoot(t)
	removeMappingValue(t, requireMappingValue(t, root, "jobs"), "deploy_homebrew_tap")
	err := checkWorkflow(mustMarshalYAML(t, root))
	if err == nil || !strings.Contains(err.Error(), "workflow jobs") {
		t.Fatalf("policy error = %v, want missing Homebrew tap publication gate rejection", err)
	}
}

func TestCheckWorkflowRejectsSecurityAndQualityPolicyMutations(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name   string
		want   string
		mutate func(t *testing.T, root *yaml.Node)
	}{
		{
			name: "action tag instead of SHA",
			want: "full 40-character lowercase SHA",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				uses := findFirstUses(t, root)
				uses.Value = "actions/checkout@v4"
			},
		},
		{
			name: "no actions",
			want: "at least one action",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				removeMappingKeyRecursively(root, "uses")
			},
		},
		{
			name: "action uses is not a scalar SHA",
			want: "full 40-character lowercase SHA",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				uses := findFirstUses(t, root)
				uses.Kind = yaml.MappingNode
				uses.Tag = "!!map"
				uses.Value = ""
				uses.Content = nil
			},
		},
		{
			name: "global permission grant",
			want: "global permissions must be an empty mapping",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				permissions := requireMappingValue(t, root, "permissions")
				permissions.Content = []*yaml.Node{scalarNode("contents"), scalarNode("write")}
			},
		},
		{
			name: "validate job permission elevated",
			want: "permissions must contain only contents: read",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				requireMappingValue(t, requireMappingValue(t, jobNode(t, root, "validate"), "permissions"), "contents").Value = "write"
			},
		},
		{
			name: "validate job declares release environment",
			want: "validate job must not declare an environment",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				appendMappingValue(t, jobNode(t, root, "validate"), "environment", scalarNode("release"))
			},
		},
		{
			name: "build job declares release environment",
			want: "build job must not declare an environment",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				appendMappingValue(t, jobNode(t, root, "build"), "environment", scalarNode("release"))
			},
		},
		{
			name: "verify job declares release environment",
			want: "verify_release_source job must not declare an environment",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				appendMappingValue(t, jobNode(t, root, "verify_release_source"), "environment", scalarNode("release"))
			},
		},
		{
			name: "publish bypasses verify source dependency",
			want: "needs must equal \"verify_release_source\"",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				requireMappingValue(t, jobNode(t, root, "publish"), "needs").Value = "build"
			},
		},
		{
			name: "tag filter broadened",
			want: "on.push.tags must equal [v[0-9]*]",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				tags := requireMappingValue(t, requireMappingValue(t, requireMappingValue(t, root, "on"), "push"), "tags")
				tags.Content[0].Value = "v*"
			},
		},
		{
			name: "pull request trigger",
			want: "on must contain only push and workflow_dispatch triggers",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				on := requireMappingValue(t, root, "on")
				on.Content = append(on.Content, scalarNode("pull_request"), &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"})
			},
		},
		{
			name: "push branch trigger",
			want: "on.push must contain only tags",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				push := requireMappingValue(t, requireMappingValue(t, root, "on"), "push")
				appendMappingValue(t, push, "branches", sequenceNode("main"))
			},
		},
		{
			name: "push path trigger",
			want: "on.push must contain only tags",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				push := requireMappingValue(t, requireMappingValue(t, root, "on"), "push")
				appendMappingValue(t, push, "paths", sequenceNode("**"))
			},
		},
		{
			name: "matrix runner changed",
			want: "build matrix must contain exactly the six release targets",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				include := requireMappingValue(t, requireMappingValue(t, requireMappingValue(t, jobNode(t, root, "build"), "strategy"), "matrix"), "include")
				requireMappingValue(t, include.Content[0], "runner").Value = "ubuntu-latest"
			},
		},
		{
			name: "release environment changed",
			want: "publish environment must be release",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				requireMappingValue(t, jobNode(t, root, "publish"), "environment").Value = "unprotected"
			},
		},
		{
			name: "default branch ancestry removed",
			want: "verify_release_source job must contain a default-branch ancestry gate",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				removeCommand(t, stepWithRun(t, jobNode(t, root, "verify_release_source"), "git merge-base --is-ancestor"), "git merge-base --is-ancestor HEAD \"origin/$DEFAULT_BRANCH\"")
			},
		},
		{
			name: "default branch ancestry is conditionally skipped",
			want: "verify_release_source default-branch ancestry gate must not define if or continue-on-error",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				step := stepWithRun(t, jobNode(t, root, "verify_release_source"), "git merge-base --is-ancestor")
				appendMappingValue(t, step, "if", scalarNode("false"))
			},
		},
		{
			name: "signing secret before trust gate",
			want: "non-release job must not reference secrets",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				steps := requireMappingValue(t, jobNode(t, root, "validate"), "steps")
				steps.Content[0].Content = append(steps.Content[0].Content,
					scalarNode("env"), &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{
						scalarNode("EARLY_SIGNING_REFERENCE"), scalarNode("${{ secrets.RELEASE_SIGNING_PRIVATE_KEY }}"),
					}},
				)
			},
		},
		{
			name: "verify job secret has no expression whitespace",
			want: "non-release job must not reference secrets",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				steps := requireMappingValue(t, jobNode(t, root, "verify_release_source"), "steps")
				appendMappingValue(t, steps.Content[0], "env", &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{
					scalarNode("EARLY_SIGNING_REFERENCE"), scalarNode("${{secrets.RELEASE_SIGNING_PRIVATE_KEY}}"),
				}})
			},
		},
		{
			name: "validate job secret uses bracket expression",
			want: "non-release job must not reference secrets",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				steps := requireMappingValue(t, jobNode(t, root, "validate"), "steps")
				appendMappingValue(t, steps.Content[0], "env", &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{
					scalarNode("EARLY_SIGNING_REFERENCE"), scalarNode("${{ secrets['RELEASE_SIGNING_PRIVATE_KEY'] }}"),
				}})
			},
		},
		{
			name: "validate job serializes the bare secrets context",
			want: "non-release job must not reference secrets",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				steps := requireMappingValue(t, jobNode(t, root, "validate"), "steps")
				appendMappingValue(t, steps.Content[0], "env", &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{
					scalarNode("EARLY_SIGNING_REFERENCE"), scalarNode("${{ toJSON(secrets) }}"),
				}})
			},
		},
		{
			name: "publish references secret before signing metadata",
			want: "publish job must not reference secrets outside its signing metadata step",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				steps := requireMappingValue(t, jobNode(t, root, "publish"), "steps")
				appendMappingValue(t, steps.Content[0], "env", &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{
					scalarNode("EARLY_SIGNING_REFERENCE"), scalarNode("${{secrets.RELEASE_SIGNING_PRIVATE_KEY}}"),
				}})
			},
		},
		{
			name: "signing metadata adds an unrelated secret",
			want: "signing-secret step must declare only its expected signing secrets",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				step := stepWithRun(t, jobNode(t, root, "publish"), "go run ./scripts/releaseassets finalize")
				env := requireMappingValue(t, step, "env")
				appendMappingValue(t, env, "UNRELATED_SECRET", scalarNode("${{secrets.UNRELATED_SECRET}}"))
			},
		},
		{
			name: "releaseassets channel removed",
			want: "release publishing step must classify with the direct releaseassets case expression",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				step := stepWithRun(t, jobNode(t, root, "publish"), "gh release create")
				replaceRunFragment(t, step, "case \"$(go run ./scripts/releaseassets channel --version \"${RELEASE_TAG#v}\")\" in", "case stable in")
			},
		},
		{
			name: "releaseassets channel is unrelated to release creation",
			want: "release publishing step must classify with the direct releaseassets case expression",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				publish := jobNode(t, root, "publish")
				step := stepWithRun(t, publish, "gh release create")
				replaceRunFragment(t, step, "case \"$(go run ./scripts/releaseassets channel --version \"${RELEASE_TAG#v}\")\" in", "case \"$(printf stable)\" in")
				appendRunStep(t, publish, "Run an unrelated channel command", "go run ./scripts/releaseassets channel --version \"${RELEASE_TAG#v}\"")
			},
		},
		{
			name: "release prerelease flag is hard coded",
			want: "release publishing step must bind releaseassets channel to the prerelease flag",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				step := stepWithRun(t, jobNode(t, root, "publish"), "gh release create")
				replaceRunFragment(t, step, "prerelease)", "stable)")
			},
		},
		{
			name: "release prerelease flag branch is removed",
			want: "release publishing step must bind releaseassets channel to the prerelease flag",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				removeCommand(t, stepWithRun(t, jobNode(t, root, "publish"), "gh release create"), "prerelease+=(--prerelease)")
			},
		},
		{
			name: "release channel stable rejection branch removed",
			want: "release publishing step must bind releaseassets channel to the prerelease flag",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				removeCommand(t, stepWithRun(t, jobNode(t, root, "publish"), "gh release create"), "*)")
			},
		},
		{
			name: "release channel result is reset before creation",
			want: "release publishing step must not hard-code or reassign the prerelease flag",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				step := stepWithRun(t, jobNode(t, root, "publish"), "gh release create")
				replaceRunFragment(t, step, "gh release create \"$RELEASE_TAG\"", "prerelease=()\n          gh release create \"$RELEASE_TAG\"")
			},
		},
		{
			name: "release channel is reassigned before classification",
			want: "release publishing step must use only the approved channel case commands",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				step := stepWithRun(t, jobNode(t, root, "publish"), "gh release create")
				replaceRunFragment(t, step, "prerelease=()", "channel=stable\n          prerelease=()")
			},
		},
		{
			name: "release channel uses printf variable rewrite",
			want: "release publishing step must use only the approved channel case commands",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				step := stepWithRun(t, jobNode(t, root, "publish"), "gh release create")
				replaceRunFragment(t, step, "prerelease=()", "printf -v channel %s stable\n          prerelease=()")
			},
		},
		{
			name: "release case contains unrelated shell code",
			want: "release publishing step must use only the approved channel case commands",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				step := stepWithRun(t, jobNode(t, root, "publish"), "gh release create")
				replaceRunFragment(t, step, "stable)", "stable)\nprintf '%s\\n' 'unrelated shell code'")
			},
		},
		{
			name: "package target argument removed",
			want: "build_production job must package the tag-bound executable",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				removeRunFragment(t, stepWithRun(t, jobNode(t, root, "build_production"), "go run ./scripts/releaseassets package"), "--target '${{ matrix.goos }}/${{ matrix.goarch }}'")
			},
		},
		{
			name: "finalize private key argument removed",
			want: "signing-secret step must contain --private-key \"$key_path\"",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				removeRunFragment(t, stepWithRun(t, jobNode(t, root, "publish"), "go run ./scripts/releaseassets finalize"), "--private-key \"$key_path\"")
			},
		},
		{
			name: "release draft flag removed",
			want: "release publishing step must contain --draft",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				removeRunFragment(t, stepWithRun(t, jobNode(t, root, "publish"), "gh release create"), "--draft")
			},
		},
		{
			name: "hyphen shell prerelease classification",
			want: "must not classify prereleases with a hyphen shell pattern",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				step := stepWithRun(t, jobNode(t, root, "publish"), "gh release create")
				run := requireMappingValue(t, step, "run")
				run.Value += "\nif [[ \"${RELEASE_TAG#v}\" == *-* ]]; then\n  prerelease+=(--prerelease)\nfi\n"
			},
		},
		{
			name: "Rust vendor gate removed",
			want: "sh scripts/test-rust-vendor.sh",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				removeCommand(t, stepWithRun(t, jobNode(t, root, "build"), "sh scripts/test-rust-vendor.sh"), "sh scripts/test-rust-vendor.sh")
			},
		},
		{
			name: "Rust format gate removed",
			want: "cargo fmt --check",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				removeCommand(t, stepWithRun(t, jobNode(t, root, "build"), "cargo fmt --check"), "cargo fmt --check")
			},
		},
		{
			name: "Rust format gate can soft fail",
			want: "build quality gate cargo fmt --check must not define continue-on-error or if",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				appendMappingValue(t, stepWithRun(t, jobNode(t, root, "build"), "cargo fmt --check"), "continue-on-error", scalarNode("true"))
			},
		},
		{
			name: "Rust format runs outside the crate",
			want: "cargo fmt --check",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				step := stepWithRun(t, jobNode(t, root, "build"), "cargo fmt --check")
				requireMappingValue(t, step, "working-directory").Value = "."
			},
		},
		{
			name: "Rust clippy gate removed",
			want: "cargo clippy --locked --offline --all-targets -- -D warnings",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				removeCommand(t, stepWithRun(t, jobNode(t, root, "build"), "cargo clippy --locked --offline --all-targets -- -D warnings"), "cargo clippy --locked --offline --all-targets -- -D warnings")
			},
		},
		{
			name: "Go test gate removed",
			want: "go test ./...",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				removeCommand(t, stepWithRun(t, jobNode(t, root, "build"), "go test ./..."), "go test ./...")
			},
		},
		{
			name: "race gate removed",
			want: "go test -race ./...",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				removeCommand(t, stepWithRun(t, jobNode(t, root, "build"), "go test -race ./..."), "go test -race ./...")
			},
		},
		{
			name: "vet gate removed",
			want: "go vet ./...",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				removeCommand(t, stepWithRun(t, jobNode(t, root, "build"), "go vet ./..."), "go vet ./...")
			},
		},
		{
			name: "license gate removed",
			want: "go run ./scripts/licensebundle --check",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				removeCommand(t, stepWithRun(t, jobNode(t, root, "build"), "go run ./scripts/licensebundle --check"), "go run ./scripts/licensebundle --check")
			},
		},
		{
			name: "package gate removed",
			want: "sh scripts/test-package-release.sh",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				removeCommand(t, stepWithRun(t, jobNode(t, root, "build"), "sh scripts/test-package-release.sh"), "sh scripts/test-package-release.sh")
			},
		},
		{
			name: "fixed pre-commit install removed",
			want: "pre-commit==4.6.0",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				removeCommand(t, stepWithRun(t, jobNode(t, root, "build"), "python -m pip install --disable-pip-version-check pre-commit==4.6.0"), "python -m pip install --disable-pip-version-check pre-commit==4.6.0")
			},
		},
		{
			name: "pre-commit run removed",
			want: "python -m pre_commit run --all-files",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				removeCommand(t, stepWithRun(t, jobNode(t, root, "build"), "python -m pre_commit run --all-files"), "python -m pre_commit run --all-files")
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			root := releaseWorkflowRoot(t)
			test.mutate(t, root)
			body, err := yaml.Marshal(root)
			if err != nil {
				t.Fatalf("marshal mutated workflow: %v", err)
			}
			err = checkWorkflow(body)
			if err == nil {
				t.Fatal("release workflow policy accepted a mutated workflow")
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Fatalf("policy error = %q, want it to mention %q", err, test.want)
			}
		})
	}
}

func TestCheckWorkflowRejectsSoftFailedOrSkippedQualityGate(t *testing.T) {
	t.Parallel()

	gates := []struct {
		name    string
		command string
	}{
		{name: "Rust format", command: "cargo fmt --check"},
		{name: "Rust clippy", command: "cargo clippy --locked --offline --all-targets -- -D warnings"},
		{name: "pre-commit install", command: "python -m pip install --disable-pip-version-check pre-commit==4.6.0"},
		{name: "pre-commit", command: "python -m pre_commit run --all-files"},
	}
	for _, gate := range gates {
		for _, mutation := range []struct {
			name  string
			key   string
			value string
		}{
			{name: "soft failure", key: "continue-on-error", value: "true"},
			{name: "conditional skip", key: "if", value: "false"},
		} {
			t.Run(gate.name+" "+mutation.name, func(t *testing.T) {
				t.Parallel()
				root := releaseWorkflowRoot(t)
				step := stepWithRun(t, jobNode(t, root, "build"), gate.command)
				appendMappingValue(t, step, mutation.key, scalarNode(mutation.value))
				body, err := yaml.Marshal(root)
				if err != nil {
					t.Fatalf("marshal mutated workflow: %v", err)
				}
				err = checkWorkflow(body)
				if err == nil || !strings.Contains(err.Error(), "must not define continue-on-error or if") {
					t.Fatalf("policy error = %v, want unconditional quality-gate rejection", err)
				}
			})
		}
	}
}

func TestCheckWorkflowAllowsOnlyDocumentedWindowsARM64RaceException(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name  string
		key   string
		value string
	}{
		{name: "condition changed", key: "if", value: "false"},
		{name: "soft failure added", key: "continue-on-error", value: "true"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			root := releaseWorkflowRoot(t)
			step := stepWithRun(t, jobNode(t, root, "build"), "go test -race ./...")
			appendMappingValue(t, step, test.key, scalarNode(test.value))
			body, err := yaml.Marshal(root)
			if err != nil {
				t.Fatalf("marshal mutated workflow: %v", err)
			}
			if err := checkWorkflow(body); err == nil {
				t.Fatal("release workflow policy accepted an unapproved race gate exception")
			}
		})
	}
}

func TestCheckWorkflowRejectsAmbiguousYAMLMutations(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name   string
		want   string
		mutate func(t *testing.T, root *yaml.Node)
	}{
		{
			name: "duplicate publish dependency",
			want: "duplicate mapping key \"needs\"",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				appendMappingValue(t, jobNode(t, root, "publish"), "needs", scalarNode("build"))
			},
		},
		{
			name: "root defaults change the working directory",
			want: "workflow root must not declare defaults",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				appendMappingValue(t, root, "defaults", mappingNode(
					"run", mappingNode("working-directory", scalarNode("/tmp")),
				))
			},
		},
		{
			name: "matrix entry has duplicate artifact",
			want: "duplicate mapping key \"artifact\"",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				include := requireMappingValue(t, requireMappingValue(t, requireMappingValue(t, jobNode(t, root, "build"), "strategy"), "matrix"), "include")
				appendMappingValue(t, include.Content[0], "artifact", scalarNode("rewritten-artifact"))
			},
		},
		{
			name: "YAML alias",
			want: "workflow must not use YAML aliases",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				checkout := findFirstUses(t, root)
				checkout.Anchor = "checkout"
				appendMappingValue(t, root, "alias", &yaml.Node{Kind: yaml.AliasNode, Value: "checkout", Alias: checkout})
			},
		},
		{
			name: "YAML merge key",
			want: "workflow must not use YAML merge keys",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				appendMappingValue(t, root, "<<", mappingNode("env", mappingNode("UNSAFE", scalarNode("true"))))
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			root := releaseWorkflowRoot(t)
			test.mutate(t, root)
			body, err := yaml.Marshal(root)
			if err != nil {
				t.Fatalf("marshal mutated workflow: %v", err)
			}
			err = checkWorkflow(body)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("policy error = %v, want ambiguous-YAML rejection %q", err, test.want)
			}
		})
	}
}

func TestCheckWorkflowRejectsRequiredJobExecutionOverrides(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name  string
		job   string
		key   string
		value *yaml.Node
		want  string
	}{
		{name: "validate if", job: "validate", key: "if", value: scalarNode("false"), want: "validate job must not define if or continue-on-error"},
		{name: "validate continue on error", job: "validate", key: "continue-on-error", value: scalarNode("true"), want: "validate job must not define if or continue-on-error"},
		{name: "build if", job: "build", key: "if", value: scalarNode("false"), want: "build job must not define if or continue-on-error"},
		{name: "build continue on error", job: "build", key: "continue-on-error", value: scalarNode("true"), want: "build job must not define if or continue-on-error"},
		{name: "verify if", job: "verify_release_source", key: "if", value: scalarNode("false"), want: "verify_release_source job must not define if or continue-on-error"},
		{name: "verify continue on error", job: "verify_release_source", key: "continue-on-error", value: scalarNode("true"), want: "verify_release_source job must not define if or continue-on-error"},
		{name: "publish if", job: "publish", key: "if", value: scalarNode("always()"), want: "publish job must not define if or continue-on-error"},
		{name: "publish continue on error", job: "publish", key: "continue-on-error", value: scalarNode("true"), want: "publish job must not define if or continue-on-error"},
		{name: "build defaults", job: "build", key: "defaults", value: mappingNode("run", mappingNode("working-directory", scalarNode("/tmp"))), want: "build job must not declare defaults"},
		{name: "verify defaults", job: "verify_release_source", key: "defaults", value: mappingNode("run", mappingNode("working-directory", scalarNode("/tmp"))), want: "verify_release_source job must not declare defaults"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			root := releaseWorkflowRoot(t)
			appendMappingValue(t, jobNode(t, root, test.job), test.key, test.value)
			body, err := yaml.Marshal(root)
			if err != nil {
				t.Fatalf("marshal mutated workflow: %v", err)
			}
			err = checkWorkflow(body)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("policy error = %v, want required-job execution rejection %q", err, test.want)
			}
		})
	}
}

func TestCheckWorkflowRejectsConditionalTrustGate(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name  string
		key   string
		value string
	}{
		{name: "if", key: "if", value: "false"},
		{name: "continue on error", key: "continue-on-error", value: "true"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			root := releaseWorkflowRoot(t)
			step := stepWithRun(t, jobNode(t, root, "verify_release_source"), "git merge-base --is-ancestor")
			appendMappingValue(t, step, test.key, scalarNode(test.value))
			body, err := yaml.Marshal(root)
			if err != nil {
				t.Fatalf("marshal mutated workflow: %v", err)
			}
			err = checkWorkflow(body)
			if err == nil || !strings.Contains(err.Error(), "default-branch ancestry gate must not define if or continue-on-error") {
				t.Fatalf("policy error = %v, want unconditional trust-gate rejection", err)
			}
		})
	}
}

func TestCheckWorkflowRequiresCredentialFreeValidateAndBuildCheckouts(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name   string
		job    string
		mutate func(t *testing.T, step *yaml.Node)
	}{
		{
			name: "validate checkout omits persist credentials",
			job:  "validate",
			mutate: func(t *testing.T, step *yaml.Node) {
				removeMappingValue(t, requireMappingValue(t, step, "with"), "persist-credentials")
			},
		},
		{
			name: "build checkout persists credentials",
			job:  "build",
			mutate: func(t *testing.T, step *yaml.Node) {
				requireMappingValue(t, requireMappingValue(t, step, "with"), "persist-credentials").Value = "true"
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			root := releaseWorkflowRoot(t)
			test.mutate(t, checkoutStep(t, jobNode(t, root, test.job)))
			body, err := yaml.Marshal(root)
			if err != nil {
				t.Fatalf("marshal mutated workflow: %v", err)
			}
			err = checkWorkflow(body)
			if err == nil || !strings.Contains(err.Error(), test.job+" job must use the canonical checkout") {
				t.Fatalf("policy error = %v, want canonical checkout rejection", err)
			}
		})
	}
}

func TestCheckWorkflowRequiresCanonicalTrustCheckout(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name   string
		mutate func(t *testing.T, checkout *yaml.Node)
	}{
		{name: "ref", mutate: func(t *testing.T, checkout *yaml.Node) {
			requireMappingValue(t, requireMappingValue(t, checkout, "with"), "ref").Value = "main"
		}},
		{name: "repository", mutate: func(t *testing.T, checkout *yaml.Node) {
			appendMappingValue(t, requireMappingValue(t, checkout, "with"), "repository", scalarNode("owner/other-repository"))
		}},
		{name: "path", mutate: func(t *testing.T, checkout *yaml.Node) {
			appendMappingValue(t, requireMappingValue(t, checkout, "with"), "path", scalarNode("other-source"))
		}},
		{name: "extra with key", mutate: func(t *testing.T, checkout *yaml.Node) {
			appendMappingValue(t, requireMappingValue(t, checkout, "with"), "fetch-tags", scalarNode("true"))
		}},
		{name: "different action SHA", mutate: func(t *testing.T, checkout *yaml.Node) {
			requireMappingValue(t, checkout, "uses").Value = "actions/checkout@0000000000000000000000000000000000000000"
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			root := releaseWorkflowRoot(t)
			test.mutate(t, checkoutStep(t, jobNode(t, root, "verify_release_source")))
			body, err := yaml.Marshal(root)
			if err != nil {
				t.Fatalf("marshal mutated workflow: %v", err)
			}
			err = checkWorkflow(body)
			if err == nil || !strings.Contains(err.Error(), "verify_release_source job must use the canonical checkout") {
				t.Fatalf("policy error = %v, want canonical trust-checkout rejection", err)
			}
		})
	}
}

func TestCheckWorkflowRequiresCanonicalPublishCheckout(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name  string
		key   string
		value string
	}{
		{name: "ref", key: "ref", value: "main"},
		{name: "repository", key: "repository", value: "owner/other-repository"},
		{name: "path", key: "path", value: "other-source"},
		{name: "extra with key", key: "fetch-tags", value: "true"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			root := releaseWorkflowRoot(t)
			checkout := checkoutStep(t, jobNode(t, root, "publish"))
			with := requireMappingValue(t, checkout, "with")
			if value, ok := mappingValue(with, test.key); ok {
				value.Value = test.value
			} else {
				appendMappingValue(t, with, test.key, scalarNode(test.value))
			}
			body, err := yaml.Marshal(root)
			if err != nil {
				t.Fatalf("marshal mutated workflow: %v", err)
			}
			err = checkWorkflow(body)
			if err == nil || !strings.Contains(err.Error(), "publish job must use the canonical checkout") {
				t.Fatalf("policy error = %v, want canonical publish-checkout rejection", err)
			}
		})
	}
}

func TestCheckWorkflowRequiresCanonicalValidateAndBuildCheckouts(t *testing.T) {
	t.Parallel()

	for _, jobName := range []string{"validate", "build"} {
		for _, test := range []struct {
			name  string
			key   string
			value string
		}{
			{name: "ref", key: "ref", value: "main"},
			{name: "repository", key: "repository", value: "owner/other-repository"},
			{name: "path", key: "path", value: "other-source"},
			{name: "extra with key", key: "fetch-tags", value: "true"},
		} {
			t.Run(jobName+" "+test.name, func(t *testing.T) {
				t.Parallel()
				root := releaseWorkflowRoot(t)
				checkout := checkoutStep(t, jobNode(t, root, jobName))
				with := requireMappingValue(t, checkout, "with")
				if value, ok := mappingValue(with, test.key); ok {
					value.Value = test.value
				} else {
					appendMappingValue(t, with, test.key, scalarNode(test.value))
				}
				body, err := yaml.Marshal(root)
				if err != nil {
					t.Fatalf("marshal mutated workflow: %v", err)
				}
				err = checkWorkflow(body)
				if err == nil || !strings.Contains(err.Error(), jobName+" job must use the canonical checkout") {
					t.Fatalf("policy error = %v, want canonical %s checkout rejection", err, jobName)
				}
			})
		}
	}
}

func TestCheckWorkflowAllowsOnlyCanonicalTrustSteps(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name   string
		mutate func(t *testing.T, job *yaml.Node)
	}{
		{name: "checkout followed by source switch", mutate: func(t *testing.T, job *yaml.Node) {
			insertRunStep(t, job, 1, "Replace the checked-out tag", "git checkout main")
		}},
		{name: "step after ancestry", mutate: func(t *testing.T, job *yaml.Node) {
			appendRunStep(t, job, "Run after trust gate", "git status --short")
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			root := releaseWorkflowRoot(t)
			test.mutate(t, jobNode(t, root, "verify_release_source"))
			body, err := yaml.Marshal(root)
			if err != nil {
				t.Fatalf("marshal mutated workflow: %v", err)
			}
			err = checkWorkflow(body)
			if err == nil || !strings.Contains(err.Error(), "verify_release_source job must contain only the canonical checkout and ancestry gate steps") {
				t.Fatalf("policy error = %v, want exact trust-step sequence rejection", err)
			}
		})
	}
}

func TestCheckWorkflowRejectsNonDirectQualityGateRuns(t *testing.T) {
	t.Parallel()

	for _, mutation := range []struct {
		name string
		run  func(string) string
	}{
		{name: "conditional shell", run: func(gate string) string { return "if false; then\n  " + gate + "\nfi" }},
		{name: "softened with or true", run: func(gate string) string { return gate + " || true" }},
	} {
		for _, gate := range requiredQualityGateCommands() {
			t.Run(gate+" "+mutation.name, func(t *testing.T) {
				t.Parallel()
				root := releaseWorkflowRoot(t)
				step := stepWithRun(t, jobNode(t, root, "build"), gate)
				requireMappingValue(t, step, "run").Value = mutation.run(gate)
				body, err := yaml.Marshal(root)
				if err != nil {
					t.Fatalf("marshal mutated workflow: %v", err)
				}
				err = checkWorkflow(body)
				if err == nil || !strings.Contains(err.Error(), gate) {
					t.Fatalf("policy error = %v, want direct quality-gate rejection for %q", err, gate)
				}
			})
		}
	}

	t.Run("unrelated control flow", func(t *testing.T) {
		t.Parallel()
		root := releaseWorkflowRoot(t)
		step := stepWithRun(t, jobNode(t, root, "build"), "go test ./...")
		requireMappingValue(t, step, "run").Value = "go test ./...\nif true; then\n  :\nfi"
		body, err := yaml.Marshal(root)
		if err != nil {
			t.Fatalf("marshal mutated workflow: %v", err)
		}
		err = checkWorkflow(body)
		if err == nil || !strings.Contains(err.Error(), "go test ./...") {
			t.Fatalf("policy error = %v, want direct quality-gate rejection", err)
		}
	})
}

func TestCheckWorkflowRejectsQualityGateExecutionOverrides(t *testing.T) {
	t.Parallel()

	for _, gate := range requiredQualityGateCommands() {
		for _, mutation := range []struct {
			name   string
			mutate func(t *testing.T, step *yaml.Node)
		}{
			{name: "environment", mutate: func(t *testing.T, step *yaml.Node) {
				appendMappingValue(t, step, "env", mappingNode("PWD", scalarNode("/tmp")))
			}},
			{name: "defaults", mutate: func(t *testing.T, step *yaml.Node) {
				appendMappingValue(t, step, "defaults", mappingNode("run", mappingNode("working-directory", scalarNode("/tmp"))))
			}},
			{name: "shell override", mutate: func(t *testing.T, step *yaml.Node) { requireMappingValue(t, step, "shell").Value = "bash -e {0}" }},
		} {
			t.Run(gate+" "+mutation.name, func(t *testing.T) {
				t.Parallel()
				root := releaseWorkflowRoot(t)
				step := stepWithRun(t, jobNode(t, root, "build"), gate)
				mutation.mutate(t, step)
				body, err := yaml.Marshal(root)
				if err != nil {
					t.Fatalf("marshal mutated workflow: %v", err)
				}
				err = checkWorkflow(body)
				if err == nil || !strings.Contains(err.Error(), gate) {
					t.Fatalf("policy error = %v, want quality execution-override rejection for %q", err, gate)
				}
			})
		}
	}
}

func TestCheckWorkflowRejectsBracketSecretExpressionsOutsideExpectedSigningEnvironment(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name   string
		want   string
		mutate func(t *testing.T, root *yaml.Node)
	}{
		{
			name: "validate double quoted bracket",
			want: "non-release job must not reference secrets",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				appendBracketSecretToFirstStep(t, jobNode(t, root, "validate"), "${{secrets[\"RELEASE_SIGNING_PRIVATE_KEY\"]}}")
			},
		},
		{
			name: "build single quoted bracket",
			want: "non-release job must not reference secrets",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				appendBracketSecretToFirstStep(t, jobNode(t, root, "build"), "${{ secrets['RELEASE_SIGNING_PRIVATE_KEY'] }}")
			},
		},
		{
			name: "verify double quoted bracket",
			want: "non-release job must not reference secrets",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				appendBracketSecretToFirstStep(t, jobNode(t, root, "verify_release_source"), "${{ secrets[\"RELEASE_SIGNING_PRIVATE_KEY\"] }}")
			},
		},
		{
			name: "publish pre-signing single quoted bracket",
			want: "publish job must not reference secrets outside its signing metadata step",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				appendBracketSecretToFirstStep(t, jobNode(t, root, "publish"), "${{secrets['RELEASE_SIGNING_PRIVATE_KEY']}}")
			},
		},
		{
			name: "signing extra double quoted bracket",
			want: "signing-secret step must declare only its expected signing secrets",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				step := stepWithRun(t, jobNode(t, root, "publish"), "go run ./scripts/releaseassets finalize")
				appendMappingValue(t, requireMappingValue(t, step, "env"), "UNRELATED_SECRET", scalarNode("${{secrets[\"UNRELATED_SECRET\"]}}"))
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			root := releaseWorkflowRoot(t)
			test.mutate(t, root)
			body, err := yaml.Marshal(root)
			if err != nil {
				t.Fatalf("marshal mutated workflow: %v", err)
			}
			err = checkWorkflow(body)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("policy error = %v, want bracket-secret rejection %q", err, test.want)
			}
		})
	}
}

func TestCheckWorkflowRejectsBareAndFunctionSecretContextsOutsideExpectedSigningEnvironment(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name   string
		want   string
		mutate func(t *testing.T, root *yaml.Node)
	}{
		{
			name: "validate bare secrets",
			want: "non-release job must not reference secrets",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				appendBracketSecretToFirstStep(t, jobNode(t, root, "validate"), "${{ secrets }}")
			},
		},
		{
			name: "build serializes secrets",
			want: "non-release job must not reference secrets",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				appendBracketSecretToFirstStep(t, jobNode(t, root, "build"), "${{toJSON(secrets)}}")
			},
		},
		{
			name: "verify bare secrets",
			want: "non-release job must not reference secrets",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				appendBracketSecretToFirstStep(t, jobNode(t, root, "verify_release_source"), "${{ secrets }}")
			},
		},
		{
			name: "publish pre-signing serializes secrets",
			want: "publish job must not reference secrets outside its signing metadata step",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				appendBracketSecretToFirstStep(t, jobNode(t, root, "publish"), "${{ toJSON(secrets) }}")
			},
		},
		{
			name: "signing key serializes secrets",
			want: "signing-secret step must use the protected private-key secret",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				step := stepWithRun(t, jobNode(t, root, "publish"), "go run ./scripts/releaseassets finalize")
				requireMappingValue(t, requireMappingValue(t, step, "env"), "RELEASE_SIGNING_PRIVATE_KEY").Value = "${{ toJSON(secrets) }}"
			},
		},
		{
			name: "signing extra bare secrets",
			want: "signing-secret step must declare only its expected signing secrets",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				step := stepWithRun(t, jobNode(t, root, "publish"), "go run ./scripts/releaseassets finalize")
				appendMappingValue(t, requireMappingValue(t, step, "env"), "UNRELATED_SECRET", scalarNode("${{ secrets }}"))
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			root := releaseWorkflowRoot(t)
			test.mutate(t, root)
			body, err := yaml.Marshal(root)
			if err != nil {
				t.Fatalf("marshal mutated workflow: %v", err)
			}
			err = checkWorkflow(body)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("policy error = %v, want bare/function-secret rejection %q", err, test.want)
			}
		})
	}
}

func TestCheckWorkflowAcceptsCanonicalBracketSigningExpressions(t *testing.T) {
	t.Parallel()

	root := releaseWorkflowRoot(t)
	step := stepWithRun(t, jobNode(t, root, "publish"), "go run ./scripts/releaseassets finalize")
	env := requireMappingValue(t, step, "env")
	requireMappingValue(t, env, "RELEASE_SIGNING_PRIVATE_KEY").Value = "${{ secrets['RELEASE_SIGNING_PRIVATE_KEY'] }}"
	requireMappingValue(t, env, "RELEASE_SIGNING_KEY_ID").Value = "${{secrets[\"RELEASE_SIGNING_KEY_ID\"]}}"
	body, err := yaml.Marshal(root)
	if err != nil {
		t.Fatalf("marshal bracket-signing workflow: %v", err)
	}
	if err := checkWorkflow(body); err != nil {
		t.Fatalf("policy rejected canonical bracket signing expressions: %v", err)
	}
}

func releaseWorkflowRoot(t *testing.T) *yaml.Node {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(findRepositoryRoot(t), ".github", "workflows", "release.yml"))
	if err != nil {
		t.Fatalf("read release workflow: %v", err)
	}
	var document yaml.Node
	if err := yaml.Unmarshal(body, &document); err != nil {
		t.Fatalf("parse release workflow: %v", err)
	}
	if document.Kind != yaml.DocumentNode || len(document.Content) != 1 {
		t.Fatal("release workflow must have exactly one document")
	}
	return document.Content[0]
}

func mustMarshalYAML(t *testing.T, node *yaml.Node) []byte {
	t.Helper()
	body, err := yaml.Marshal(node)
	if err != nil {
		t.Fatalf("marshal workflow fixture: %v", err)
	}
	return body
}

func jobNode(t *testing.T, root *yaml.Node, name string) *yaml.Node {
	t.Helper()
	return requireMappingValue(t, requireMappingValue(t, root, "jobs"), name)
}

func requireMappingValue(t *testing.T, mapping *yaml.Node, key string) *yaml.Node {
	t.Helper()
	value, ok := mappingValue(mapping, key)
	if !ok {
		t.Fatalf("mapping has no %q key", key)
	}
	return value
}

func findFirstUses(t *testing.T, node *yaml.Node) *yaml.Node {
	t.Helper()
	if node.Kind == yaml.MappingNode {
		for index := 0; index+1 < len(node.Content); index += 2 {
			if node.Content[index].Value == "uses" {
				return node.Content[index+1]
			}
			if found := findFirstUses(t, node.Content[index+1]); found != nil {
				return found
			}
		}
	}
	for _, child := range node.Content {
		if found := findFirstUses(t, child); found != nil {
			return found
		}
	}
	return nil
}

func removeMappingKeyRecursively(node *yaml.Node, key string) {
	if node.Kind == yaml.MappingNode {
		kept := make([]*yaml.Node, 0, len(node.Content))
		for index := 0; index+1 < len(node.Content); index += 2 {
			if node.Content[index].Value == key {
				continue
			}
			removeMappingKeyRecursively(node.Content[index+1], key)
			kept = append(kept, node.Content[index], node.Content[index+1])
		}
		node.Content = kept
		return
	}
	for _, child := range node.Content {
		removeMappingKeyRecursively(child, key)
	}
}

func stepWithRun(t *testing.T, job *yaml.Node, command string) *yaml.Node {
	t.Helper()
	steps := requireMappingValue(t, job, "steps")
	for _, step := range steps.Content {
		run, ok := mappingValue(step, "run")
		if ok && strings.Contains(run.Value, command) {
			return step
		}
	}
	t.Fatalf("job has no step running %q", command)
	return nil
}

func checkoutStep(t *testing.T, job *yaml.Node) *yaml.Node {
	t.Helper()
	steps := requireMappingValue(t, job, "steps")
	for _, step := range steps.Content {
		uses, ok := mappingValue(step, "uses")
		if ok && strings.HasPrefix(uses.Value, "actions/checkout@") {
			return step
		}
	}
	t.Fatal("job has no checkout step")
	return nil
}

func removeCommand(t *testing.T, step *yaml.Node, command string) {
	t.Helper()
	run := requireMappingValue(t, step, "run")
	lines := strings.Split(run.Value, "\n")
	kept := make([]string, 0, len(lines))
	removed := false
	for _, line := range lines {
		if strings.TrimSpace(line) == command {
			removed = true
			continue
		}
		kept = append(kept, line)
	}
	if !removed {
		t.Fatalf("step did not run %q", command)
	}
	run.Value = strings.Join(kept, "\n")
}

func removeRunFragment(t *testing.T, step *yaml.Node, fragment string) {
	t.Helper()
	run := requireMappingValue(t, step, "run")
	if !strings.Contains(run.Value, fragment) {
		t.Fatalf("step did not contain %q", fragment)
	}
	run.Value = strings.Replace(run.Value, fragment, "", 1)
}

func replaceRunFragment(t *testing.T, step *yaml.Node, old, new string) {
	t.Helper()
	run := requireMappingValue(t, step, "run")
	if !strings.Contains(run.Value, old) {
		t.Fatalf("step did not contain %q", old)
	}
	run.Value = strings.Replace(run.Value, old, new, 1)
}

func appendRunStep(t *testing.T, job *yaml.Node, name, run string) {
	t.Helper()
	steps := requireMappingValue(t, job, "steps")
	steps.Content = append(steps.Content, runStepNode(name, run))
}

func insertRunStep(t *testing.T, job *yaml.Node, index int, name, run string) {
	t.Helper()
	steps := requireMappingValue(t, job, "steps")
	if index < 0 || index > len(steps.Content) {
		t.Fatalf("step index %d is outside the workflow", index)
	}
	steps.Content = append(steps.Content, nil)
	copy(steps.Content[index+1:], steps.Content[index:])
	steps.Content[index] = runStepNode(name, run)
}

func runStepNode(name, run string) *yaml.Node {
	return &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{
		scalarNode("name"), scalarNode(name),
		scalarNode("shell"), scalarNode("bash"),
		scalarNode("run"), scalarNode(run),
	}}
}

func appendMappingValue(t *testing.T, mapping *yaml.Node, key string, value *yaml.Node) {
	t.Helper()
	if mapping.Kind != yaml.MappingNode {
		t.Fatal("append mapping value to a non-mapping node")
	}
	mapping.Content = append(mapping.Content, scalarNode(key), value)
}

func removeMappingValue(t *testing.T, mapping *yaml.Node, key string) {
	t.Helper()
	if mapping.Kind != yaml.MappingNode {
		t.Fatal("remove mapping value from a non-mapping node")
	}
	for index := 0; index+1 < len(mapping.Content); index += 2 {
		if mapping.Content[index].Value != key {
			continue
		}
		mapping.Content = append(mapping.Content[:index], mapping.Content[index+2:]...)
		return
	}
	t.Fatalf("mapping has no %q key", key)
}

func appendBracketSecretToFirstStep(t *testing.T, job *yaml.Node, expression string) {
	t.Helper()
	steps := requireMappingValue(t, job, "steps")
	appendMappingValue(t, steps.Content[0], "env", &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{
		scalarNode("UNEXPECTED_SECRET"), scalarNode(expression),
	}})
}

func scalarNode(value string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
}

func sequenceNode(values ...string) *yaml.Node {
	node := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	for _, value := range values {
		node.Content = append(node.Content, scalarNode(value))
	}
	return node
}

func mappingNode(entries ...any) *yaml.Node {
	if len(entries)%2 != 0 {
		panic("mappingNode requires key-value pairs")
	}
	node := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	for index := 0; index < len(entries); index += 2 {
		key, ok := entries[index].(string)
		if !ok {
			panic("mappingNode keys must be strings")
		}
		value, ok := entries[index+1].(*yaml.Node)
		if !ok {
			panic("mappingNode values must be YAML nodes")
		}
		node.Content = append(node.Content, scalarNode(key), value)
	}
	return node
}

func requiredQualityGateCommands() []string {
	return []string{
		"go run ./scripts/releaseassets validate --version \"${RELEASE_TAG#v}\"",
		"sh scripts/test-rust-vendor.sh",
		"cargo fmt --check",
		"cargo clippy --locked --offline --all-targets -- -D warnings",
		"go test ./...",
		"go test -race ./...",
		"go vet ./...",
		"go run ./scripts/licensebundle --check",
		"sh scripts/test-package-release.sh",
		"python -m pip install --disable-pip-version-check pre-commit==4.6.0",
		"python -m pre_commit run --all-files",
	}
}

func findRepositoryRoot(t *testing.T) string {
	t.Helper()
	directory, err := os.Getwd()
	if err != nil {
		t.Fatalf("get current directory: %v", err)
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
