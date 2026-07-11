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
			want: "default-branch ancestry gate",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				removeCommand(t, stepWithRun(t, jobNode(t, root, "publish"), "git merge-base --is-ancestor"), "git merge-base --is-ancestor HEAD \"origin/$DEFAULT_BRANCH\"")
			},
		},
		{
			name: "signing secret before trust gate",
			want: "signing secret is reachable before the default-branch ancestry gate",
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
			name: "releaseassets channel removed",
			want: "releaseassets channel",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				removeCommand(t, stepWithRun(t, jobNode(t, root, "publish"), "channel=$(go run ./scripts/releaseassets channel"), "channel=$(go run ./scripts/releaseassets channel --version \"${GITHUB_REF_NAME#v}\")")
			},
		},
		{
			name: "package target argument removed",
			want: "build packaging step must contain --target",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				removeRunFragment(t, stepWithRun(t, jobNode(t, root, "build"), "go run ./scripts/releaseassets package"), "--target '${{ matrix.goos }}/${{ matrix.goarch }}'")
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
				step := stepWithRun(t, jobNode(t, root, "publish"), "channel=$(go run ./scripts/releaseassets channel")
				run := requireMappingValue(t, step, "run")
				run.Value += "\nif [[ \"${GITHUB_REF_NAME#v}\" == *-* ]]; then\n  prerelease+=(--prerelease)\nfi\n"
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
		{
			name: "diff check removed",
			want: "git diff --check",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				removeCommand(t, stepWithRun(t, jobNode(t, root, "build"), "git diff --check"), "git diff --check")
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

func scalarNode(value string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
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
