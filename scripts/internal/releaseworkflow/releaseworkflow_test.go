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

	root := releaseWorkflowRoot(t)
	removeMappingValue(t, requireMappingValue(t, root, "jobs"), "build_production")
	if err := checkWorkflow(mustMarshalYAML(t, root)); err == nil {
		t.Fatal("release workflow policy accepted a missing production build job")
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

	root := releaseWorkflowRoot(t)
	insertRunStep(t, jobNode(t, root, "build"), 14, "Forbidden production build", "go build -trimpath ./cmd/pixiv")
	if err := checkWorkflow(mustMarshalYAML(t, root)); err == nil {
		t.Fatal("release workflow policy accepted a production build command in the test job")
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

	root := releaseWorkflowRoot(t)
	findFirstUses(t, root).Value = "actions/checkout@v4"
	if err := checkWorkflow(mustMarshalYAML(t, root)); err == nil {
		t.Fatal("release workflow policy accepted an unpinned action")
	}
}

func TestCheckWorkflowAllowsOnlyCanonicalTrustSteps(t *testing.T) {
	t.Parallel()

	root := releaseWorkflowRoot(t)
	insertRunStep(t, jobNode(t, root, "verify_release_source"), 1, "Replace the checked-out tag", "git checkout main")
	if err := checkWorkflow(mustMarshalYAML(t, root)); err == nil {
		t.Fatal("release workflow policy accepted an extra trust step")
	}
}

func TestCheckWorkflowRejectsBracketSecretExpressionsOutsideExpectedSigningEnvironment(t *testing.T) {
	t.Parallel()

	root := releaseWorkflowRoot(t)
	appendBracketSecretToFirstStep(t, jobNode(t, root, "validate"), "${{ secrets[\"RELEASE_SIGNING_PRIVATE_KEY\"] }}")
	if err := checkWorkflow(mustMarshalYAML(t, root)); err == nil {
		t.Fatal("release workflow policy accepted a bracket secret outside the signing step")
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
