package main

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/scripts/internal/workflowpolicy"
	"gopkg.in/yaml.v3"
)

var actionReferencePattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+@[0-9a-f]{40}$`)

var documentationOnlyPathIgnores = []string{
	"README*.md",
	"docs/**",
	"changelog/**",
	"skills/**",
}

const canonicalCheckoutAction = "actions/checkout@34e114876b0b11c390a56381ad16ebd13914f8d5"

const canonicalSetupGoAction = "actions/setup-go@40f1582b2485089dde7abd97c1529aa768e1baff"

const canonicalUploadArtifactAction = "actions/upload-artifact@ea165f8d65b6e75b540449e92b4886f43607fa02"

// checkWorkflow 先拒绝发布凭据与副作用，再验证该入口只能从 main 或人工 dispatch 启动。
// 后续的 evidence 记录与回填同样依赖这个可在本地执行的 fail-closed policy。
func checkWorkflow(body []byte) error {
	var document yaml.Node
	if err := yaml.Unmarshal(body, &document); err != nil {
		return fmt.Errorf("parse YAML: %w", err)
	}
	if err := workflowpolicy.RejectAmbiguousYAML(&document); err != nil {
		return err
	}
	if document.Kind != yaml.DocumentNode || len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return errors.New("workflow must contain exactly one mapping document")
	}
	root := document.Content[0]
	if err := requireOnlyMappingKeys(root, "name", "on", "permissions", "jobs"); err != nil {
		return errors.New("native evidence workflow root must contain only its audited fields")
	}
	if workflowpolicy.ContainsSecretReference(root) {
		return errors.New("native evidence workflow must not reference secrets")
	}
	if err := checkNativeEvidenceTrigger(root); err != nil {
		return err
	}
	if err := checkEmptyPermissions(root); err != nil {
		return err
	}
	jobs, ok := workflowpolicy.MappingValue(root, "jobs")
	if !ok || jobs.Kind != yaml.MappingNode || len(jobs.Content) != 2 {
		return errors.New("native evidence workflow must have exactly one job")
	}
	jobName, job := jobs.Content[0], jobs.Content[1]
	if jobName.Value != "native_evidence" || job.Kind != yaml.MappingNode {
		return errors.New("native evidence workflow must have a native_evidence job")
	}
	if err := checkActionReferences(root); err != nil {
		return err
	}
	if err := checkNoReleaseSideEffects(root); err != nil {
		return err
	}
	return checkNativeEvidenceJob(job)
}

func checkNativeEvidenceJob(job *yaml.Node) error {
	if err := requireOnlyMappingKeys(job, "name", "runs-on", "permissions", "env", "strategy", "steps"); err != nil {
		return errors.New("native evidence job must contain only its audited fields")
	}
	if err := workflowpolicy.RequireScalar(job, "runs-on", "${{ matrix.runner }}"); err != nil {
		return errors.New("native evidence job must run only on its audited matrix runner")
	}
	if err := checkContentsReadPermission(job); err != nil {
		return err
	}
	env, ok := workflowpolicy.MappingValue(job, "env")
	if !ok || requireOnlyMappingKeys(env, "RUSTUP_TOOLCHAIN") != nil || workflowpolicy.RequireScalar(env, "RUSTUP_TOOLCHAIN", "${{ matrix.rust_toolchain }}") != nil {
		return errors.New("native evidence job must bind the Rust toolchain from its audited matrix")
	}
	strategy, ok := workflowpolicy.MappingValue(job, "strategy")
	if !ok || strategy.Kind != yaml.MappingNode || requireOnlyMappingKeys(strategy, "fail-fast", "matrix") != nil || workflowpolicy.RequireScalar(strategy, "fail-fast", "false") != nil {
		return errors.New("native evidence job must have a fail-fast false matrix strategy")
	}
	matrix, ok := workflowpolicy.MappingValue(strategy, "matrix")
	if !ok || matrix.Kind != yaml.MappingNode {
		return errors.New("native evidence job must have a matrix")
	}
	if err := checkNativeEvidenceMatrix(matrix); err != nil {
		return err
	}
	steps, ok := workflowpolicy.MappingValue(job, "steps")
	if !ok || steps.Kind != yaml.SequenceNode || len(steps.Content) != 12 {
		return errors.New("native evidence job must contain its complete audited step sequence")
	}
	if err := requireCanonicalCheckout(steps.Content[0]); err != nil {
		return err
	}
	if err := requireCanonicalSetupGo(steps.Content[1]); err != nil {
		return err
	}
	if err := requireDirectRunStep(steps.Content[2], "Check native evidence workflow policy", "go run ./scripts/nativeevidence policy --workflow .github/workflows/native-evidence.yml"); err != nil {
		return err
	}
	if err := requireDirectRunStep(steps.Content[3], "Require audited main ref", "test \"$GITHUB_REF\" = 'refs/heads/main'"); err != nil {
		return err
	}
	if err := requireDirectRunStep(steps.Content[4], "Install the pinned native Rust toolchain", "rustup toolchain install '${{ matrix.rust_toolchain }}' --profile minimal --target '${{ matrix.rust_target }}' --no-self-update"); err != nil {
		return err
	}
	if err := requireDirectRunStep(steps.Content[5], "Check vendored Rust sources", "sh scripts/test-rust-vendor.sh"); err != nil {
		return err
	}
	if err := requireDirectRunStep(steps.Content[6], "Build the native Rust static library", "bash scripts/build-staticlibs.sh --target '${{ matrix.rust_target }}'"); err != nil {
		return err
	}
	if err := requireMultilineRunStep(steps.Content[7], "Run native GIF/APNG smoke", canonicalNativeSmokeRun); err != nil {
		return err
	}
	if err := requireMultilineRunStep(steps.Content[8], "Build and run the versioned native binary", canonicalBuildBinaryRun); err != nil {
		return err
	}
	if err := requireMultilineRunStep(steps.Content[9], "Package the versioned native binary", canonicalPackageRun); err != nil {
		return err
	}
	if err := requireMultilineRunStep(steps.Content[10], "Record staticlib, binary and archive evidence", canonicalRecordRun); err != nil {
		return err
	}
	if err := requireUploadEvidenceStep(steps.Content[11]); err != nil {
		return err
	}
	return nil
}

var nativeEvidenceMatrixTargets = map[string]struct{}{
	"macos-15-intel|darwin|amd64|x86_64-apple-darwin|darwin-amd64":       {},
	"macos-15|darwin|arm64|aarch64-apple-darwin|darwin-arm64":            {},
	"ubuntu-22.04|linux|amd64|x86_64-unknown-linux-gnu|linux-amd64":      {},
	"ubuntu-22.04-arm|linux|arm64|aarch64-unknown-linux-gnu|linux-arm64": {},
	"windows-2025|windows|amd64|x86_64-pc-windows-msvc|windows-amd64":    {},
	"windows-11-arm|windows|arm64|aarch64-pc-windows-msvc|windows-arm64": {},
}

func checkNativeEvidenceMatrix(matrix *yaml.Node) error {
	if requireOnlyMappingKeys(matrix, "include") != nil {
		return errors.New("native evidence matrix must contain only its six audited targets")
	}
	include, ok := workflowpolicy.MappingValue(matrix, "include")
	if !ok || include.Kind != yaml.SequenceNode || len(include.Content) != len(nativeEvidenceMatrixTargets) {
		return errors.New("native evidence matrix must contain exactly the six audited targets")
	}
	seen := make(map[string]struct{}, len(include.Content))
	for _, entry := range include.Content {
		if entry.Kind != yaml.MappingNode || requireOnlyMappingKeys(entry, "runner", "goos", "goarch", "rust_target", "rust_toolchain", "artifact") != nil {
			return errors.New("native evidence matrix must contain exactly the six audited targets")
		}
		target, _ := workflowpolicy.MappingValue(entry, "rust_target")
		toolchain, _ := workflowpolicy.MappingValue(entry, "rust_toolchain")
		wantToolchain, supported := workflowpolicy.PinnedRustToolchain(target.Value)
		if !supported || toolchain.Value != wantToolchain {
			return errors.New("native evidence matrix must use the release-pinned Rust toolchain for every target")
		}
		parts := make([]string, 0, 5)
		for _, key := range []string{"runner", "goos", "goarch", "rust_target", "artifact"} {
			value, ok := workflowpolicy.MappingValue(entry, key)
			if !ok || value.Kind != yaml.ScalarNode {
				return errors.New("native evidence matrix must contain exactly the six audited targets")
			}
			parts = append(parts, value.Value)
		}
		identity := strings.Join(parts, "|")
		if _, ok := nativeEvidenceMatrixTargets[identity]; !ok {
			return errors.New("native evidence matrix must contain exactly the six audited targets")
		}
		if _, duplicate := seen[identity]; duplicate {
			return errors.New("native evidence matrix must contain exactly the six audited targets")
		}
		seen[identity] = struct{}{}
	}
	return nil
}

func requireCanonicalCheckout(step *yaml.Node) error {
	if requireOnlyMappingKeys(step, "uses", "with") != nil || workflowpolicy.RequireScalar(step, "uses", canonicalCheckoutAction) != nil {
		return errors.New("native evidence job must use the canonical credential-free checkout")
	}
	with, ok := workflowpolicy.MappingValue(step, "with")
	if !ok || requireOnlyMappingKeys(with, "persist-credentials") != nil || workflowpolicy.RequireScalar(with, "persist-credentials", "false") != nil {
		return errors.New("native evidence job must use the canonical credential-free checkout")
	}
	return nil
}

func requireCanonicalSetupGo(step *yaml.Node) error {
	if requireOnlyMappingKeys(step, "uses", "with") != nil || workflowpolicy.RequireScalar(step, "uses", canonicalSetupGoAction) != nil {
		return errors.New("native evidence job must use the canonical Go setup action")
	}
	with, ok := workflowpolicy.MappingValue(step, "with")
	if !ok || requireOnlyMappingKeys(with, "go-version") != nil || workflowpolicy.RequireScalar(with, "go-version", "1.26.3") != nil {
		return errors.New("native evidence job must use the canonical Go setup action")
	}
	return nil
}

func requireDirectRunStep(step *yaml.Node, name, command string) error {
	if requireOnlyMappingKeys(step, "name", "shell", "run") != nil || workflowpolicy.RequireScalar(step, "name", name) != nil || workflowpolicy.RequireScalar(step, "shell", "bash") != nil || workflowpolicy.RequireScalar(step, "run", command) != nil {
		return fmt.Errorf("native evidence job must retain direct step %q", name)
	}
	return nil
}

const canonicalNativeSmokeRun = `set -eu
if [ '${{ matrix.goos }}' = windows ]; then
  export CC='clang -fuse-ld=lld'
fi
go test ./internal/ugoira -run '^TestRustUgoiraEncoderNativeGIFAndAPNG$' -count=1
`

const canonicalBuildBinaryRun = `set -eu
if [ '${{ matrix.goos }}' = windows ]; then
  export CC='clang -fuse-ld=lld'
fi
version="0.1.0-native-evidence.${GITHUB_RUN_ID}"
go run ./scripts/releaseassets validate --version "$version"
mkdir -p evidence
binary='evidence/pixiv'
if [ '${{ matrix.goos }}' = windows ]; then
  binary='evidence/pixiv.exe'
fi
go build -trimpath -buildvcs=false \
  -ldflags "-X github.com/FlanChanXwO/pixiv-cli/internal/buildinfo.Version=v${version} -X github.com/FlanChanXwO/pixiv-cli/internal/buildinfo.Commit=${GITHUB_SHA} -X github.com/FlanChanXwO/pixiv-cli/internal/buildinfo.BuildDate=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  -o "$binary" ./cmd/pixiv
if [ '${{ matrix.goos }}' = linux ]; then
  go run ./scripts/linuxabi --binary "$binary"
fi
"$binary" version --json
`

const canonicalPackageRun = `set -eu
version="0.1.0-native-evidence.${GITHUB_RUN_ID}"
binary='evidence/pixiv'
if [ '${{ matrix.goos }}' = windows ]; then
  binary='evidence/pixiv.exe'
fi
go run ./scripts/releaseassets package \
  --repo-root . \
  --version "$version" \
  --target '${{ matrix.goos }}/${{ matrix.goarch }}' \
  --binary "$binary" \
  --output-dir evidence
`

const canonicalRecordRun = `set -eu
version="0.1.0-native-evidence.${GITHUB_RUN_ID}"
staticlib='libugoira_rs.a'
if [ '${{ matrix.goos }}' = windows ]; then
  staticlib='ugoira_rs.lib'
fi
cp "internal/downloader/ugoira_rs/staticlib/${{ matrix.rust_target }}/$staticlib" "evidence/$staticlib"
archive="evidence/pixiv-cli_${version}_${{ matrix.goos }}_${{ matrix.goarch }}"
if [ '${{ matrix.goos }}' = windows ]; then
  archive="$archive.zip"
else
  archive="$archive.tar.gz"
fi
binary='evidence/pixiv'
if [ '${{ matrix.goos }}' = windows ]; then
  binary='evidence/pixiv.exe'
fi
go run ./scripts/nativeevidence record \
  --repo-root . \
  --version "$version" \
  --target '${{ matrix.goos }}/${{ matrix.goarch }}' \
  --rust-target '${{ matrix.rust_target }}' \
  --staticlib "evidence/$staticlib" \
  --binary "$binary" \
  --archive "$archive" \
  --output evidence/native-evidence.json
`

func requireMultilineRunStep(step *yaml.Node, name, canonical string) error {
	if requireOnlyMappingKeys(step, "name", "shell", "run") != nil || workflowpolicy.RequireScalar(step, "name", name) != nil || workflowpolicy.RequireScalar(step, "shell", "bash") != nil {
		return fmt.Errorf("native evidence job must retain step %q", name)
	}
	run, ok := workflowpolicy.MappingValue(step, "run")
	if !ok || run.Kind != yaml.ScalarNode || run.Value != canonical {
		return fmt.Errorf("native evidence job must retain guarded step %q", name)
	}
	return nil
}

func requireUploadEvidenceStep(step *yaml.Node) error {
	if requireOnlyMappingKeys(step, "name", "uses", "with") != nil || workflowpolicy.RequireScalar(step, "name", "Upload native evidence") != nil || workflowpolicy.RequireScalar(step, "uses", canonicalUploadArtifactAction) != nil {
		return errors.New("native evidence job must upload only its audited evidence artifact")
	}
	with, ok := workflowpolicy.MappingValue(step, "with")
	if !ok || requireOnlyMappingKeys(with, "name", "path", "if-no-files-found") != nil || workflowpolicy.RequireScalar(with, "name", "native-evidence-${{ matrix.artifact }}") != nil || workflowpolicy.RequireScalar(with, "path", "evidence") != nil || workflowpolicy.RequireScalar(with, "if-no-files-found", "error") != nil {
		return errors.New("native evidence job must upload only its audited evidence artifact")
	}
	return nil
}

func checkNativeEvidenceTrigger(root *yaml.Node) error {
	on, ok := workflowpolicy.MappingValue(root, "on")
	if !ok || on.Kind != yaml.MappingNode {
		return errors.New("native evidence workflow must have an on mapping")
	}
	push, hasPush := workflowpolicy.MappingValue(on, "push")
	dispatch, hasDispatch := workflowpolicy.MappingValue(on, "workflow_dispatch")
	if !hasPush || !hasDispatch || len(on.Content) != 4 || push.Kind != yaml.MappingNode || dispatch.Kind != yaml.MappingNode || len(dispatch.Content) != 0 {
		return errors.New("native evidence workflow must use only main push and workflow_dispatch triggers")
	}
	branches, hasBranches := workflowpolicy.MappingValue(push, "branches")
	pathIgnores, hasPathIgnores := workflowpolicy.MappingValue(push, "paths-ignore")
	if !hasBranches || !hasPathIgnores || len(push.Content) != 4 || branches.Kind != yaml.SequenceNode || len(branches.Content) != 1 || branches.Content[0].Value != "main" {
		return errors.New("native evidence workflow push trigger must be limited to main and its audited documentation ignores")
	}
	if pathIgnores.Kind != yaml.SequenceNode || len(pathIgnores.Content) != len(documentationOnlyPathIgnores) {
		return errors.New("native evidence workflow must ignore exactly the audited documentation paths")
	}
	for index, want := range documentationOnlyPathIgnores {
		if pathIgnores.Content[index].Kind != yaml.ScalarNode || pathIgnores.Content[index].Value != want {
			return errors.New("native evidence workflow must ignore exactly the audited documentation paths")
		}
	}
	return nil
}

func checkEmptyPermissions(root *yaml.Node) error {
	permissions, ok := workflowpolicy.MappingValue(root, "permissions")
	if !ok || permissions.Kind != yaml.MappingNode || len(permissions.Content) != 0 {
		return errors.New("native evidence workflow global permissions must be an empty mapping")
	}
	return nil
}

func checkContentsReadPermission(job *yaml.Node) error {
	permissions, ok := workflowpolicy.MappingValue(job, "permissions")
	if !ok || permissions.Kind != yaml.MappingNode || len(permissions.Content) != 2 {
		return errors.New("native evidence job permissions must contain only contents: read")
	}
	value, ok := workflowpolicy.MappingValue(permissions, "contents")
	if !ok || value.Kind != yaml.ScalarNode || value.Value != "read" {
		return errors.New("native evidence job permissions must contain only contents: read")
	}
	return nil
}

func checkActionReferences(root *yaml.Node) error {
	var references []string
	collectActionReferences(root, &references)
	if len(references) == 0 {
		return errors.New("native evidence workflow must use at least one action")
	}
	for _, reference := range references {
		if !actionReferencePattern.MatchString(reference) {
			return errors.New("every action uses reference must be a full 40-character lowercase SHA")
		}
	}
	return nil
}

func collectActionReferences(node *yaml.Node, references *[]string) {
	if node == nil {
		return
	}
	if node.Kind == yaml.MappingNode {
		for index := 0; index+1 < len(node.Content); index += 2 {
			key, value := node.Content[index], node.Content[index+1]
			if key.Value == "uses" && value.Kind == yaml.ScalarNode {
				*references = append(*references, value.Value)
			}
			collectActionReferences(value, references)
		}
		return
	}
	for _, child := range node.Content {
		collectActionReferences(child, references)
	}
}

func checkNoReleaseSideEffects(node *yaml.Node) error {
	var values []string
	collectScalarValues(node, &values)
	for _, value := range values {
		for _, forbidden := range []string{"gh release", "git push", "git tag", "releaseassets finalize", "HOMEBREW_TAP", "RELEASE_SIGNING", "github.token", "GITHUB_TOKEN", "GH_TOKEN", "curl", "wget"} {
			if regexp.MustCompile(`(?i)` + regexp.QuoteMeta(forbidden)).MatchString(value) {
				return fmt.Errorf("native evidence workflow must not contain release side effect %q", forbidden)
			}
		}
	}
	return nil
}

func requireOnlyMappingKeys(mapping *yaml.Node, keys ...string) error {
	if !workflowpolicy.HasExactMappingKeys(mapping, keys...) {
		return errors.New("must contain exactly the audited keys")
	}
	return nil
}

func collectScalarValues(node *yaml.Node, values *[]string) {
	if node == nil {
		return
	}
	if node.Kind == yaml.ScalarNode {
		*values = append(*values, node.Value)
	}
	for _, child := range node.Content {
		collectScalarValues(child, values)
	}
}
