package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/scripts/internal/workflowpolicy"
	"gopkg.in/yaml.v3"
)

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

func TestCheckWorkflowRejectsFormattedSigningSecretBeforeSigningMetadata(t *testing.T) {
	t.Parallel()

	root := releaseWorkflowRoot(t)
	publish := jobNode(t, root, "publish")
	steps := requireMappingValue(t, publish, "steps")
	signingStep := stepWithRun(t, publish, "go run ./scripts/releaseassets finalize")
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
			name: "finalize Unix installer argument removed",
			want: "signing-secret step must contain --install-sh scripts/install.sh",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				removeRunFragment(t, stepWithRun(t, jobNode(t, root, "publish"), "go run ./scripts/releaseassets finalize"), "--install-sh scripts/install.sh")
			},
		},
		{
			name: "finalize Windows installer argument removed",
			want: "signing-secret step must contain --install-cmd scripts/install.cmd",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				removeRunFragment(t, stepWithRun(t, jobNode(t, root, "publish"), "go run ./scripts/releaseassets finalize"), "--install-cmd scripts/install.cmd")
			},
		},
		{
			name: "finalize release-source argument removed",
			want: "signing-secret step must contain --release-sources internal/update/release_sources.txt",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				removeRunFragment(t, stepWithRun(t, jobNode(t, root, "publish"), "go run ./scripts/releaseassets finalize"), "--release-sources internal/update/release_sources.txt")
			},
		},
		{
			name: "finalize Simplified Chinese changelog argument removed",
			want: "signing-secret step must contain --changelog-zh \"changelog/v${RELEASE_TAG#v}/zh-CN.md\"",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				removeRunFragment(t, stepWithRun(t, jobNode(t, root, "publish"), "go run ./scripts/releaseassets finalize"), "--changelog-zh \"changelog/v${RELEASE_TAG#v}/zh-CN.md\"")
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
			if value, ok := workflowpolicy.MappingValue(with, test.key); ok {
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
				if value, ok := workflowpolicy.MappingValue(with, test.key); ok {
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
