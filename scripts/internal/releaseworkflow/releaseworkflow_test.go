package releaseworkflow

import (
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
	"strings"
	"testing"

	workflowyaml "github.com/FlanChanXwO/pixiv-cli/scripts/internal/workflow/yaml"
)

func TestCheckWorkflowAcceptsCheckedInWorkflow(t *testing.T) {
	t.Parallel()

	body, err := os.ReadFile(filepath.Join(findRepositoryRoot(t), ".github", "workflows", "release.yml"))
	if err != nil {
		t.Fatalf("read release workflow: %v", err)
	}
	if err := checkWorkflow(body); err != nil {
		t.Fatalf("release workflow policy rejected checked-in workflow: %v", err)
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
			toolchain, ok := workflowyaml.MappingValue(env, "RUSTUP_TOOLCHAIN")
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

	payload, err := os.ReadFile("../../../.github/workflows/release.yml")
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(payload)
	for _, required := range []string{
		"runner: ubuntu-22.04\n            goos: linux\n            goarch: amd64",
		"runner: ubuntu-22.04-arm\n            goos: linux\n            goarch: arm64",
		`go run ./scripts/cmd/linuxabi --binary "$output"`,
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
		uses, ok := workflowyaml.MappingValue(step, "uses")
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
		{name: "production runs quality gate", mutate: func(t *testing.T, root *yaml.Node) {
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
			needs := requireMappingValue(t, jobNode(t, root, "verify_release_source"), "needs")
			needs.Kind = yaml.ScalarNode
			needs.Tag = "!!str"
			needs.Value = "build"
			needs.Content = nil
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

func TestProductionBuildEmbedsOnlyRootVersion(t *testing.T) {
	t.Parallel()

	root := releaseWorkflowRoot(t)
	step := stepWithRun(t, jobNode(t, root, "build_production"), "go build -trimpath")
	run := requireMappingValue(t, step, "run").Value
	if !strings.Contains(run, `-ldflags "-X github.com/FlanChanXwO/pixiv-cli/internal/shared/buildinfo.Version=${RELEASE_TAG}"`) {
		t.Fatal("production build must bind the root version to the immutable release tag")
	}
	for _, forbidden := range []string{"buildinfo.Commit", "buildinfo.BuildDate"} {
		if strings.Contains(run, forbidden) {
			t.Fatalf("production build retains removed runtime metadata contract %q", forbidden)
		}
	}
}

// Windows 的 Go+cgo 质量门和最终 binary 必须统一使用 Clang+LLD，不能只修某个 step。
func TestCheckWorkflowRequiresWindowsClangLLDForWholeBuildJob(t *testing.T) {
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
	if err := checkWorkflow(mustMarshalYAML(t, root)); err == nil || !strings.Contains(err.Error(), "build matrix entries must contain only the canonical release target fields") {
		t.Fatalf("policy error = %v, want Windows Clang+LLD rejection", err)
	}
}

func TestCheckWorkflowRequiresUnconditionalDedicatedSemVerValidatorStep(t *testing.T) {
	t.Parallel()

	const validator = `go run ./scripts/cmd/releaseassets validate --version "${RELEASE_TAG#v}"`
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
		{
			name: "validator failure is ignored",
			mutate: func(t *testing.T, step *yaml.Node) {
				replaceRunFragment(t, step, validator, validator+" || true")
			},
		},
		{
			name: "validator is bound to a literal version",
			mutate: func(t *testing.T, step *yaml.Node) {
				replaceRunFragment(t, step, validator, `go run ./scripts/cmd/releaseassets validate --version "1.2.3"`)
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

func TestCheckWorkflowRequiresProductSkillVersionValidationFromReleaseTag(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name   string
		mutate func(step *yaml.Node)
	}{
		{
			name: "product skill temp path removed",
			mutate: func(step *yaml.Node) {
				replaceRunFragment(t, step,
					`tag_skill="$RUNNER_TEMP/pixiv-cli-SKILL.md"`,
					`product_skill_path="$RUNNER_TEMP/pixiv-cli-SKILL.md"`,
				)
			},
		},
		{
			name: "product skill blob replaced",
			mutate: func(step *yaml.Node) {
				replaceRunFragment(t, step,
					`git show "$RELEASE_TAG:skills/pixiv-cli/SKILL.md" > "$tag_skill"`,
					`git show "$RELEASE_TAG:README.md" > "$tag_skill"`,
				)
			},
		},
		{
			name: "product skill version binding removed",
			mutate: func(step *yaml.Node) {
				replaceRunFragment(t, step,
					`go run ./scripts/cmd/releaseassets validate-source --version "${RELEASE_TAG#v}" --product-skill "$tag_skill"`,
					`go run ./scripts/cmd/releaseassets validate-source --version "1.2.3" --product-skill "$tag_skill"`,
				)
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := releaseWorkflowRoot(t)
			step := stepWithRun(t, jobNode(t, root, "validate"), `git show "$RELEASE_TAG:skills/pixiv-cli/SKILL.md" > "$tag_skill"`)
			test.mutate(step)
			if err := checkWorkflow(mustMarshalYAML(t, root)); err == nil || !strings.Contains(err.Error(), "validate release tag step") {
				t.Fatalf("policy error = %v, want product skill version validation rejection", err)
			}
		})
	}
}

func TestCheckWorkflowRejectsBuildQualityMutations(t *testing.T) {
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
		{name: "license gate deleted", mutate: func(t *testing.T, root *yaml.Node) {
			steps := requireMappingValue(t, jobNode(t, root, "build"), "steps")
			index := stepIndexWithRunFragment(steps.Content, "go run ./scripts/cmd/licensebundle --check")
			steps.Content = append(steps.Content[:index], steps.Content[index+1:]...)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := releaseWorkflowRoot(t)
			test.mutate(t, root)
			if err := checkWorkflow(mustMarshalYAML(t, root)); err == nil {
				t.Fatal("release workflow policy accepted a build quality mutation")
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
func TestCheckWorkflowRejectsFormattedSigningSecretBeforeSigningMetadata(t *testing.T) {
	t.Parallel()

	root := releaseWorkflowRoot(t)
	publish := jobNode(t, root, "publish")
	steps := requireMappingValue(t, publish, "steps")
	signingStep := stepWithRun(t, publish, "go run ./scripts/cmd/releaseassets finalize")
	signingIndex := -1
	for index, step := range steps.Content {
		if step == signingStep {
			signingIndex = index
			break
		}
	}
	if signingIndex < 0 {
		t.Fatal("publish job has no signing metadata step")
	}
	insertRunStep(t, publish, signingIndex, "Read signing secret early", "printf '%s\\n' \"$EARLY_SIGNING_REFERENCE\"")
	appendMappingValue(t, steps.Content[signingIndex], "env", mappingNode(
		"EARLY_SIGNING_REFERENCE", scalarNode(`${{ format('{0}', secrets.RELEASE_SIGNING_PRIVATE_KEY) }}`),
	))

	body, err := yaml.Marshal(root)
	if err != nil {
		t.Fatalf("marshal mutated workflow: %v", err)
	}
	err = checkWorkflow(body)
	want := "publish job must not reference secrets outside its signing metadata step"
	if err == nil || err.Error() != want {
		t.Fatalf("policy error = %v, want %q", err, want)
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
			want: "verified container",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				// 正式 workflow 现在用 sequence 同时声明容器 artifact 与 trust gate；
				// 改成 scalar 才能表达“只依赖 build、绕过 verify_release_source”。
				needs := requireMappingValue(t, jobNode(t, root, "publish"), "needs")
				needs.Kind = yaml.ScalarNode
				needs.Tag = "!!str"
				needs.Value = "build"
				needs.Content = nil
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
			want: "on must contain only the push trigger",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				on := requireMappingValue(t, root, "on")
				on.Content = append(on.Content, scalarNode("pull_request"), &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"})
			},
		},
		{
			name: "manual recovery trigger",
			want: "on must contain only the push trigger",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				on := requireMappingValue(t, root, "on")
				on.Content = append(on.Content, scalarNode("workflow_dispatch"), &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"})
			},
		},
		{
			name: "release tag binding changed",
			want: "workflow must bind RELEASE_TAG only to the pushed tag",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				requireMappingValue(t, requireMappingValue(t, root, "env"), "RELEASE_TAG").Value = "${{ github.sha }}"
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
				step := stepWithRun(t, jobNode(t, root, "publish"), "go run ./scripts/cmd/releaseassets finalize")
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
				replaceRunFragment(t, step, "case \"$(go run ./scripts/cmd/releaseassets channel --version \"${RELEASE_TAG#v}\")\" in", "case stable in")
			},
		},
		{
			name: "releaseassets channel is unrelated to release creation",
			want: "release publishing step must classify with the direct releaseassets case expression",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				publish := jobNode(t, root, "publish")
				step := stepWithRun(t, publish, "gh release create")
				replaceRunFragment(t, step, "case \"$(go run ./scripts/cmd/releaseassets channel --version \"${RELEASE_TAG#v}\")\" in", "case \"$(printf stable)\" in")
				appendRunStep(t, publish, "Run an unrelated channel command", "go run ./scripts/cmd/releaseassets channel --version \"${RELEASE_TAG#v}\"")
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
				removeRunFragment(t, stepWithRun(t, jobNode(t, root, "build_production"), "go run ./scripts/cmd/releaseassets package"), "--target '${{ matrix.goos }}/${{ matrix.goarch }}'")
			},
		},
		{
			name: "finalize private key argument removed",
			want: "signing-secret step must contain --private-key \"$key_path\"",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				removeRunFragment(t, stepWithRun(t, jobNode(t, root, "publish"), "go run ./scripts/cmd/releaseassets finalize"), "--private-key \"$key_path\"")
			},
		},
		{
			name: "finalize Unix installer argument removed",
			want: "signing-secret step must contain --install-sh scripts/install.sh",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				removeRunFragment(t, stepWithRun(t, jobNode(t, root, "publish"), "go run ./scripts/cmd/releaseassets finalize"), "--install-sh scripts/install.sh")
			},
		},
		{
			name: "finalize Windows installer argument removed",
			want: "signing-secret step must contain --install-cmd scripts/install.cmd",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				removeRunFragment(t, stepWithRun(t, jobNode(t, root, "publish"), "go run ./scripts/cmd/releaseassets finalize"), "--install-cmd scripts/install.cmd")
			},
		},
		{
			name: "finalize release-source argument removed",
			want: "signing-secret step must contain --release-sources internal/update/source/release_sources.txt",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				removeRunFragment(t, stepWithRun(t, jobNode(t, root, "publish"), "go run ./scripts/cmd/releaseassets finalize"), "--release-sources internal/update/source/release_sources.txt")
			},
		},
		{
			name: "finalize Simplified Chinese changelog argument removed",
			want: "signing-secret step must contain --changelog-zh \"changelog/v${RELEASE_TAG#v}/zh-CN.md\"",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				removeRunFragment(t, stepWithRun(t, jobNode(t, root, "publish"), "go run ./scripts/cmd/releaseassets finalize"), "--changelog-zh \"changelog/v${RELEASE_TAG#v}/zh-CN.md\"")
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
			name: "Unix installer release asset removed",
			want: "release publishing step must preserve the verified asset set",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				removeRunFragment(t, stepWithRun(t, jobNode(t, root, "publish"), "gh release create"), "release/install.sh")
			},
		},
		{
			name: "Windows installer release asset removed",
			want: "release publishing step must preserve the verified asset set",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				removeRunFragment(t, stepWithRun(t, jobNode(t, root, "publish"), "gh release create"), "release/install.cmd")
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
			want: "go run ./scripts/cmd/licensebundle --check",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				removeCommand(t, stepWithRun(t, jobNode(t, root, "build"), "go run ./scripts/cmd/licensebundle --check"), "go run ./scripts/cmd/licensebundle --check")
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
			if value, ok := workflowyaml.MappingValue(with, test.key); ok {
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
				if value, ok := workflowyaml.MappingValue(with, test.key); ok {
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
				step := stepWithRun(t, jobNode(t, root, "publish"), "go run ./scripts/cmd/releaseassets finalize")
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
				step := stepWithRun(t, jobNode(t, root, "publish"), "go run ./scripts/cmd/releaseassets finalize")
				requireMappingValue(t, requireMappingValue(t, step, "env"), "RELEASE_SIGNING_PRIVATE_KEY").Value = "${{ toJSON(secrets) }}"
			},
		},
		{
			name: "signing extra bare secrets",
			want: "signing-secret step must declare only its expected signing secrets",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				step := stepWithRun(t, jobNode(t, root, "publish"), "go run ./scripts/cmd/releaseassets finalize")
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
	step := stepWithRun(t, jobNode(t, root, "publish"), "go run ./scripts/cmd/releaseassets finalize")
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
	value, ok := workflowyaml.MappingValue(mapping, key)
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
		run, ok := workflowyaml.MappingValue(step, "run")
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
		uses, ok := workflowyaml.MappingValue(step, "uses")
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
		"go run ./scripts/cmd/releaseassets validate-source --version \"${RELEASE_TAG#v}\" --product-skill skills/pixiv-cli/SKILL.md",
		"sh scripts/test-rust-vendor.sh",
		"cargo fmt --check",
		"cargo clippy --locked --offline --all-targets -- -D warnings",
		"go test ./...",
		"go test -race ./...",
		"go vet ./...",
		"go run ./scripts/cmd/licensebundle --check",
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
