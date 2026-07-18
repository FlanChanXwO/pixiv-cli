package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/scripts/internal/workflowpolicy"
	"gopkg.in/yaml.v3"
)

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
			toolchain, ok := workflowpolicy.MappingValue(env, "RUSTUP_TOOLCHAIN")
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

func TestReleaseBuildLocksPortableLinuxABI(t *testing.T) {
	t.Parallel()

	payload, err := os.ReadFile("../../.github/workflows/release.yml")
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(payload)
	for _, required := range []string{
		"runner: ubuntu-22.04\n            goos: linux\n            goarch: amd64",
		"runner: ubuntu-22.04-arm\n            goos: linux\n            goarch: arm64",
		`go run ./scripts/linuxabi --binary "$output"`,
	} {
		if !strings.Contains(workflow, required) {
			t.Fatalf("release workflow missing Linux ABI contract %q", required)
		}
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
			want: "release-pinned Rust toolchain",
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
	cache, ok := workflowpolicy.MappingValue(with, "cache")
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
		uses, ok := workflowpolicy.MappingValue(step, "uses")
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

// 不可变 tag 已经存在时，只允许从默认分支受审计 workflow 以精确 tag 恢复；
// 生产版本、checkout 和发布命令必须继续绑定同一个 RELEASE_TAG。
func TestCheckRecoveryPolicyRequiresTrustedReleaseTag(t *testing.T) {
	t.Parallel()

	root := releaseWorkflowRoot(t)
	if err := checkRecoveryPolicy(root); err != nil {
		t.Fatalf("checked-in recovery policy rejected: %v", err)
	}
}

// v0.3.0 tag 已包含顶层 account external test；恢复只覆盖 workflow、拆分后的 verifier、
// 全部 verifier 测试和编译 verifier 必需的共享 production helper，生产资产源码仍只来自 tag。
func TestCheckRecoveryPolicyRequiresExactAuditedOverlay(t *testing.T) {
	t.Parallel()

	const commands = `set -euo pipefail
test -z "$(git status --porcelain=v1 --untracked-files=all)"
test -z "$(git diff --cached --name-only)"
git archive --format=tar "$GITHUB_SHA" -- \
  .github/workflows/release.yml \
  scripts/internal/workflowpolicy/policy.go \
  scripts/releaseworkflow/build_policy.go \
  scripts/releaseworkflow/build_recovery_test.go \
  scripts/releaseworkflow/homebrew_policy.go \
  scripts/releaseworkflow/homebrew_policy_test.go \
  scripts/releaseworkflow/main.go \
  scripts/releaseworkflow/main_test.go \
  scripts/releaseworkflow/publish_policy.go \
  scripts/releaseworkflow/publish_security_test.go \
  scripts/releaseworkflow/recovery_policy.go \
  scripts/releaseworkflow/test_helpers_test.go \
  scripts/releaseworkflow/workflow_policy.go \
  scripts/releaseworkflow/workflow_policy_test.go | tar -xf -
test "$(
  {
    git diff --name-only
    git ls-files --others --exclude-standard
  } | LC_ALL=C sort
)" = "$(printf '%s\n' \
  .github/workflows/release.yml \
  scripts/internal/workflowpolicy/policy.go \
  scripts/releaseworkflow/build_policy.go \
  scripts/releaseworkflow/build_recovery_test.go \
  scripts/releaseworkflow/homebrew_policy.go \
  scripts/releaseworkflow/homebrew_policy_test.go \
  scripts/releaseworkflow/main.go \
  scripts/releaseworkflow/main_test.go \
  scripts/releaseworkflow/publish_policy.go \
  scripts/releaseworkflow/publish_security_test.go \
  scripts/releaseworkflow/recovery_policy.go \
  scripts/releaseworkflow/test_helpers_test.go \
  scripts/releaseworkflow/workflow_policy.go \
  scripts/releaseworkflow/workflow_policy_test.go)"
test -z "$(git diff --cached --name-only)"`
	paths := []string{
		".github/workflows/release.yml",
		"scripts/internal/workflowpolicy/policy.go",
		"scripts/releaseworkflow/build_policy.go",
		"scripts/releaseworkflow/build_recovery_test.go",
		"scripts/releaseworkflow/homebrew_policy.go",
		"scripts/releaseworkflow/homebrew_policy_test.go",
		"scripts/releaseworkflow/main.go",
		"scripts/releaseworkflow/main_test.go",
		"scripts/releaseworkflow/publish_policy.go",
		"scripts/releaseworkflow/publish_security_test.go",
		"scripts/releaseworkflow/recovery_policy.go",
		"scripts/releaseworkflow/test_helpers_test.go",
		"scripts/releaseworkflow/workflow_policy.go",
		"scripts/releaseworkflow/workflow_policy_test.go",
	}
	root := releaseWorkflowRoot(t)
	step := stepWithRun(t, jobNode(t, root, "build"), `git archive --format=tar "$GITHUB_SHA"`)
	run := requireMappingValue(t, step, "run")
	if run.Value != commands+"\n" {
		t.Fatalf("recovery overlay command = %q, want exact audited command", run.Value)
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
	t.Run("arbitrary extra path added", func(t *testing.T) {
		root := releaseWorkflowRoot(t)
		step := stepWithRun(t, jobNode(t, root, "build"), `git archive --format=tar "$GITHUB_SHA"`)
		run := requireMappingValue(t, step, "run")
		run.Value = strings.ReplaceAll(run.Value, "  scripts/releaseworkflow/main.go \\", "  pkg/pixiv/other_test.go \\\n  scripts/releaseworkflow/main.go \\")
		if err := checkRecoveryPolicy(root); err == nil {
			t.Fatal("release recovery policy accepted an extra overlay path")
		}
	})
}

// TestRecoveryOverlayExecutesAgainstV030TrackedBaseline 使用真实 shell 验证旧 tag
// 只跟踪三个既有路径时，新增 verifier 文件仍参与精确 allowlist 核对。
func TestRecoveryOverlayExecutesAgainstV030TrackedBaseline(t *testing.T) {
	root := releaseWorkflowRoot(t)
	step := stepWithRun(t, jobNode(t, root, "build"), `git archive --format=tar "$GITHUB_SHA"`)
	run := requireMappingValue(t, step, "run").Value

	t.Run("exact overlay succeeds", func(t *testing.T) {
		repository, overlayCommit := newRecoveryOverlayRepository(t)
		command := newRecoveryShellCommand(repository, run, overlayCommit)
		output, err := command.CombinedOutput()
		if err != nil {
			tracked := runGitCommand(t, repository, "diff", "--name-only")
			untracked := runGitCommand(t, repository, "ls-files", "--others", "--exclude-standard")
			t.Fatalf("execute canonical recovery overlay: %v\n%s\ntracked diff:\n%s\nuntracked files omitted by old comparison:\n%s", err, output, tracked, untracked)
		}
	})

	t.Run("dirty untracked file is rejected before extraction", func(t *testing.T) {
		repository, overlayCommit := newRecoveryOverlayRepository(t)
		unexpected := filepath.Join(repository, "unexpected.txt")
		if err := os.WriteFile(unexpected, []byte("dirty\n"), 0o644); err != nil {
			t.Fatalf("write unexpected untracked file: %v", err)
		}
		command := newRecoveryShellCommand(repository, run, overlayCommit)
		output, err := command.CombinedOutput()
		if err == nil {
			t.Fatalf("canonical recovery overlay accepted a dirty worktree:\n%s", output)
		}
		if _, err := os.Stat(filepath.Join(repository, "scripts", "releaseworkflow", "build_policy.go")); !os.IsNotExist(err) {
			t.Fatalf("recovery overlay extracted files before rejecting dirty worktree: %v", err)
		}
	})
}

func TestIsolatedGitEnvironmentRemovesRepositoryLocalStateAndUniquelyOverridesExtras(t *testing.T) {
	localNames := []string{
		"GIT_ALTERNATE_OBJECT_DIRECTORIES",
		"GIT_CONFIG",
		"GIT_CONFIG_PARAMETERS",
		"GIT_CONFIG_COUNT",
		"GIT_OBJECT_DIRECTORY",
		"GIT_DIR",
		"GIT_WORK_TREE",
		"GIT_IMPLICIT_WORK_TREE",
		"GIT_GRAFT_FILE",
		"GIT_INDEX_FILE",
		"GIT_NO_REPLACE_OBJECTS",
		"GIT_REPLACE_REF_BASE",
		"GIT_PREFIX",
		"GIT_SHALLOW_FILE",
		"GIT_COMMON_DIR",
		"GIT_INTERNAL_SUPER_PREFIX",
	}
	input := []string{"PATH=/test/bin", "HOME=/test/home", "KEEP=value", "GITHUB_SHA=stale", "GITHUB_SHA=duplicate"}
	for _, name := range localNames {
		input = append(input, name+"=unsafe")
	}
	got := isolatedGitEnvironment(input, map[string]string{"GITHUB_SHA": "overlay-commit"})
	wantValues := map[string]string{"PATH": "/test/bin", "HOME": "/test/home", "KEEP": "value", "GITHUB_SHA": "overlay-commit"}
	seen := make(map[string]int)
	for _, entry := range got {
		name, value, ok := strings.Cut(entry, "=")
		if !ok {
			t.Fatalf("isolated environment contains malformed entry %q", entry)
		}
		seen[name]++
		if want, expected := wantValues[name]; expected && value != want {
			t.Errorf("isolated environment %s = %q, want %q", name, value, want)
		}
	}
	for _, name := range localNames {
		if seen[name] != 0 {
			t.Errorf("isolated environment retained repository-local %s", name)
		}
	}
	for name := range wantValues {
		if seen[name] != 1 {
			t.Errorf("isolated environment %s count = %d, want 1", name, seen[name])
		}
	}
}

func newRecoveryShellCommand(repository, run, overlayCommit string) *exec.Cmd {
	command := exec.Command("bash", "-c", run)
	command.Dir = repository
	command.Env = isolatedGitEnvironment(os.Environ(), map[string]string{"GITHUB_SHA": overlayCommit})
	return command
}

// isolatedGitEnvironment 只移除 `git rev-parse --local-env-vars` 报告的仓库局部状态，
// 保留 PATH、HOME 等运行依赖，避免 commit hook 把父仓库的 .git/index 注入临时 fixture。
func isolatedGitEnvironment(base []string, extra map[string]string) []string {
	overrides := make(map[string]string, len(extra))
	overrideNames := make([]string, 0, len(extra))
	for name, value := range extra {
		normalized := strings.ToUpper(name)
		overrides[normalized] = normalized + "=" + value
		overrideNames = append(overrideNames, normalized)
	}
	sort.Strings(overrideNames)

	result := make([]string, 0, len(base)+len(overrides))
	for _, entry := range base {
		name := entry
		if index := strings.IndexByte(entry, '='); index >= 0 {
			name = entry[:index]
		}
		normalized := strings.ToUpper(name)
		if isGitRepositoryLocalEnvironmentName(normalized) {
			continue
		}
		if _, overridden := overrides[normalized]; overridden {
			continue
		}
		result = append(result, entry)
	}
	for _, name := range overrideNames {
		result = append(result, overrides[name])
	}
	return result
}

func isGitRepositoryLocalEnvironmentName(name string) bool {
	switch name {
	case "GIT_ALTERNATE_OBJECT_DIRECTORIES",
		"GIT_CONFIG",
		"GIT_CONFIG_PARAMETERS",
		"GIT_CONFIG_COUNT",
		"GIT_OBJECT_DIRECTORY",
		"GIT_DIR",
		"GIT_WORK_TREE",
		"GIT_IMPLICIT_WORK_TREE",
		"GIT_GRAFT_FILE",
		"GIT_INDEX_FILE",
		"GIT_NO_REPLACE_OBJECTS",
		"GIT_REPLACE_REF_BASE",
		"GIT_PREFIX",
		"GIT_SHALLOW_FILE",
		"GIT_COMMON_DIR",
		"GIT_INTERNAL_SUPER_PREFIX":
		return true
	default:
		return false
	}
}

func newRecoveryOverlayRepository(t *testing.T) (string, string) {
	t.Helper()
	repository := t.TempDir()
	runGitCommand(t, repository, "init", "--quiet")
	runGitCommand(t, repository, "config", "user.name", "Pixiv CLI Test")
	runGitCommand(t, repository, "config", "user.email", "pixiv-cli-test@example.invalid")
	runGitCommand(t, repository, "config", "commit.gpgsign", "false")
	runGitCommand(t, repository, "config", "core.hooksPath", filepath.Join(repository, ".git", "disabled-hooks"))

	// v0.3.0 的恢复相关基线只跟踪这三个既有路径；其余 verifier 文件由 overlay 新增。
	baseline := map[string]string{
		".github/workflows/release.yml":        "baseline workflow\n",
		"scripts/releaseworkflow/main.go":      "package main\n",
		"scripts/releaseworkflow/main_test.go": "package main\n",
	}
	for path, body := range baseline {
		writeRecoveryFixtureFile(t, repository, path, []byte(body))
	}
	runGitCommand(t, repository, "add", "--", ".github/workflows/release.yml", "scripts/releaseworkflow/main.go", "scripts/releaseworkflow/main_test.go")
	runGitCommand(t, repository, "commit", "--quiet", "-m", "baseline")
	baselineCommit := runGitCommand(t, repository, "rev-parse", "HEAD")

	repositoryRoot := findRepositoryRoot(t)
	paths := []string{
		".github/workflows/release.yml",
		"scripts/internal/workflowpolicy/policy.go",
		"scripts/releaseworkflow/build_policy.go",
		"scripts/releaseworkflow/build_recovery_test.go",
		"scripts/releaseworkflow/homebrew_policy.go",
		"scripts/releaseworkflow/homebrew_policy_test.go",
		"scripts/releaseworkflow/main.go",
		"scripts/releaseworkflow/main_test.go",
		"scripts/releaseworkflow/publish_policy.go",
		"scripts/releaseworkflow/publish_security_test.go",
		"scripts/releaseworkflow/recovery_policy.go",
		"scripts/releaseworkflow/test_helpers_test.go",
		"scripts/releaseworkflow/workflow_policy.go",
		"scripts/releaseworkflow/workflow_policy_test.go",
	}
	for _, path := range paths {
		body, err := os.ReadFile(filepath.Join(repositoryRoot, filepath.FromSlash(path)))
		if err != nil {
			t.Fatalf("read recovery overlay fixture %s: %v", path, err)
		}
		writeRecoveryFixtureFile(t, repository, path, body)
	}
	arguments := append([]string{"add", "--"}, paths...)
	runGitCommand(t, repository, arguments...)
	runGitCommand(t, repository, "commit", "--quiet", "-m", "overlay")
	overlayCommit := runGitCommand(t, repository, "rev-parse", "HEAD")
	runGitCommand(t, repository, "checkout", "--quiet", "--detach", baselineCommit)
	return repository, overlayCommit
}

func writeRecoveryFixtureFile(t *testing.T, repository, path string, body []byte) {
	t.Helper()
	absolute := filepath.Join(repository, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(absolute), 0o755); err != nil {
		t.Fatalf("create recovery fixture directory: %v", err)
	}
	if err := os.WriteFile(absolute, body, 0o644); err != nil {
		t.Fatalf("write recovery fixture %s: %v", path, err)
	}
}

func runGitCommand(t *testing.T, repository string, arguments ...string) string {
	t.Helper()
	command := exec.Command("git", arguments...)
	command.Dir = repository
	command.Env = isolatedGitEnvironment(os.Environ(), nil)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(arguments, " "), err, output)
	}
	return strings.TrimSpace(string(output))
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

func TestCheckWorkflowRequiresUnconditionalDedicatedSemVerValidatorStep(t *testing.T) {
	t.Parallel()

	const validator = `go run ./scripts/releaseassets validate --version "${RELEASE_TAG#v}"`
	for _, test := range []struct {
		name   string
		mutate func(t *testing.T, step *yaml.Node)
	}{
		{
			name: "conditional step",
			mutate: func(t *testing.T, step *yaml.Node) {
				appendMappingValue(t, step, "if", scalarNode("false"))
			},
		},
		{
			name: "continue on error",
			mutate: func(t *testing.T, step *yaml.Node) {
				appendMappingValue(t, step, "continue-on-error", scalarNode("true"))
			},
		},
		{
			name: "validator inside false branch",
			mutate: func(t *testing.T, step *yaml.Node) {
				replaceRunFragment(t, step, validator, "if false; then\n  "+validator+"\nfi")
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			root := releaseWorkflowRoot(t)
			step := stepWithRun(t, jobNode(t, root, "validate"), validator)
			test.mutate(t, step)
			if err := checkWorkflow(mustMarshalYAML(t, root)); err == nil || !strings.Contains(err.Error(), "Validate release SemVer") {
				t.Fatalf("policy error = %v, want unconditional dedicated validator rejection", err)
			}
		})
	}
}

func TestCheckWorkflowRejectsBypassedValidateSemVerCommand(t *testing.T) {
	t.Parallel()

	root := releaseWorkflowRoot(t)
	step := stepWithRun(t, jobNode(t, root, "validate"), `go run ./scripts/releaseassets validate --version "${RELEASE_TAG#v}"`)
	replaceRunFragment(t, step,
		`go run ./scripts/releaseassets validate --version "${RELEASE_TAG#v}"`,
		`go run ./scripts/releaseassets validate --version "${RELEASE_TAG#v}" || true`,
	)
	if err := checkWorkflow(mustMarshalYAML(t, root)); err == nil || !strings.Contains(err.Error(), "Validate release SemVer") {
		t.Fatalf("policy error = %v, want bypassed SemVer validator rejection", err)
	}
}

func TestCheckWorkflowRequiresValidateSemVerCommandBoundToReleaseTag(t *testing.T) {
	t.Parallel()

	root := releaseWorkflowRoot(t)
	step := stepWithRun(t, jobNode(t, root, "validate"), `go run ./scripts/releaseassets validate --version "${RELEASE_TAG#v}"`)
	replaceRunFragment(t, step,
		`go run ./scripts/releaseassets validate --version "${RELEASE_TAG#v}"`,
		`go run ./scripts/releaseassets validate --version "1.2.3"`,
	)
	if err := checkWorkflow(mustMarshalYAML(t, root)); err == nil || !strings.Contains(err.Error(), "Validate release SemVer") {
		t.Fatalf("policy error = %v, want validator binding rejection", err)
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
