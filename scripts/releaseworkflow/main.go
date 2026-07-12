// Command releaseworkflow 检查发布 workflow 的结构化安全与质量门禁。
package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// actionReferencePattern 只接受远端 action 的不可变完整对象 ID，避免可移动 tag 改写发布供应链。
var actionReferencePattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+@[0-9a-f]{40}$`)

const canonicalCheckoutAction = "actions/checkout@34e114876b0b11c390a56381ad16ebd13914f8d5"

// 解析后的 YAML scalar 保留 GitHub expression。任一 `${{ ... }}` 内独立的 secrets context
// （包括 bare、toJSON(secrets)、dot 和 bracket）都视为凭据引用，不能依赖具体访问语法。
var secretReferencePattern = regexp.MustCompile(`(?is)\$\{\{[^}]*\bsecrets\b[^}]*\}\}`)

// releaseMatrixTargets 将 runner、Go 平台、Rust target 和 release asset 名称绑为同一集合，
// 防止任一字段的局部改动让六平台发布遗漏或错配。
var releaseMatrixTargets = map[string]struct{}{
	"macos-15-intel|darwin|amd64|x86_64-apple-darwin|darwin-amd64|clang":                    {},
	"macos-15|darwin|arm64|aarch64-apple-darwin|darwin-arm64|clang":                         {},
	"ubuntu-24.04|linux|amd64|x86_64-unknown-linux-gnu|linux-amd64|gcc":                     {},
	"ubuntu-24.04-arm|linux|arm64|aarch64-unknown-linux-gnu|linux-arm64|gcc":                {},
	"windows-2025|windows|amd64|x86_64-pc-windows-msvc|windows-amd64|clang -fuse-ld=lld":    {},
	"windows-11-arm|windows|arm64|aarch64-pc-windows-msvc|windows-arm64|clang -fuse-ld=lld": {},
}

// homebrewMatrixTargets 绑定四个 Homebrew 验证 runner 与其实际平台，避免仅在单一
// 架构验证 staging formula 后就向公开 tap 推送。
var homebrewMatrixTargets = map[string]struct{}{
	"macos-15-intel|darwin|amd64":  {},
	"macos-15|darwin|arm64":        {},
	"ubuntu-24.04|linux|amd64":     {},
	"ubuntu-24.04-arm|linux|arm64": {},
}

const (
	setupGoAction          = "actions/setup-go@40f1582b2485089dde7abd97c1529aa768e1baff"
	uploadArtifactAction   = "actions/upload-artifact@ea165f8d65b6e75b540449e92b4886f43607fa02"
	downloadArtifactAction = "actions/download-artifact@d3f86a106a0bac45b974a628896c90dbdf5c8093"
	githubKnownHostsLine   = "github.com ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOMqqnkVzrm0SdG6UOoqKLsabgH5C9okWi0dh2l9GKJl\n"
)

func main() {
	if err := checkWorkflowPath(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "release workflow policy: %v\n", err)
		os.Exit(1)
	}
}

func checkWorkflowPath(arguments []string) error {
	if len(arguments) != 2 || arguments[0] != "--workflow" {
		return errors.New("usage: releaseworkflow --workflow PATH")
	}
	body, err := os.ReadFile(arguments[1])
	if err != nil {
		return fmt.Errorf("read workflow: %w", err)
	}
	if err := checkWorkflow(body); err != nil {
		return err
	}
	repositoryRoot := filepath.Clean(filepath.Join(filepath.Dir(arguments[1]), "..", ".."))
	knownHosts, err := os.ReadFile(filepath.Join(repositoryRoot, "templates", "homebrew", "github.com-known-hosts"))
	if err != nil {
		return fmt.Errorf("read pinned GitHub known_hosts: %w", err)
	}
	return checkPinnedGitHubKnownHosts(knownHosts)
}

func checkPinnedGitHubKnownHosts(body []byte) error {
	if string(body) != githubKnownHostsLine {
		return errors.New("GitHub known_hosts fixture must contain only the pinned official ED25519 host key")
	}
	return nil
}

func checkWorkflow(body []byte) error {
	var document yaml.Node
	if err := yaml.Unmarshal(body, &document); err != nil {
		return fmt.Errorf("parse YAML: %w", err)
	}
	if err := rejectAmbiguousYAML(&document); err != nil {
		return err
	}
	if document.Kind != yaml.DocumentNode || len(document.Content) != 1 {
		return errors.New("workflow must contain exactly one YAML document")
	}
	root := document.Content[0]
	if root.Kind != yaml.MappingNode {
		return errors.New("workflow root must be a mapping")
	}
	if err := requireNoWorkflowExecutionOverrides(root); err != nil {
		return err
	}
	if err := checkActionReferences(root); err != nil {
		return err
	}
	if err := checkTagTrigger(root); err != nil {
		return err
	}
	if err := checkGlobalPermissions(root); err != nil {
		return err
	}
	jobs, ok := mappingValue(root, "jobs")
	if !ok || jobs.Kind != yaml.MappingNode {
		return errors.New("workflow must have a jobs mapping")
	}
	if err := requireOnlyMappingKeys(jobs, "validate", "build", "build_production", "verify_release_source", "publish", "render_homebrew_formula", "verify_homebrew_formula", "deploy_homebrew_tap"); err != nil {
		return fmt.Errorf("workflow jobs: %w", err)
	}
	validate, ok := mappingValue(jobs, "validate")
	if !ok || validate.Kind != yaml.MappingNode {
		return errors.New("workflow must have a validate job")
	}
	build, ok := mappingValue(jobs, "build")
	if !ok || build.Kind != yaml.MappingNode {
		return errors.New("workflow must have a build job")
	}
	productionBuild, ok := mappingValue(jobs, "build_production")
	if !ok || productionBuild.Kind != yaml.MappingNode {
		return errors.New("workflow must have a build_production job")
	}
	verifyReleaseSource, ok := mappingValue(jobs, "verify_release_source")
	if !ok || verifyReleaseSource.Kind != yaml.MappingNode {
		return errors.New("workflow must have a verify_release_source job")
	}
	publish, ok := mappingValue(jobs, "publish")
	if !ok || publish.Kind != yaml.MappingNode {
		return errors.New("workflow must have a publish job")
	}
	renderHomebrew, ok := mappingValue(jobs, "render_homebrew_formula")
	if !ok || renderHomebrew.Kind != yaml.MappingNode {
		return errors.New("workflow must have a render_homebrew_formula job")
	}
	verifyHomebrew, ok := mappingValue(jobs, "verify_homebrew_formula")
	if !ok || verifyHomebrew.Kind != yaml.MappingNode {
		return errors.New("workflow must have a verify_homebrew_formula job")
	}
	deployHomebrew, ok := mappingValue(jobs, "deploy_homebrew_tap")
	if !ok || deployHomebrew.Kind != yaml.MappingNode {
		return errors.New("workflow must have a deploy_homebrew_tap job")
	}
	// 先报告任何越界 secret 引用，避免后续 checkout 形状校验掩盖真实的凭据泄露风险。
	preflightPublishSteps, _ := jobSteps(publish)
	preflightSigningIndex, _ := signingStepIndex(preflightPublishSteps)
	if err := checkSigningSecretReachability(validate, build, productionBuild, verifyReleaseSource, publish, preflightPublishSteps, preflightSigningIndex); err != nil {
		return err
	}
	if err := checkHomebrewSecretReachability(renderHomebrew, verifyHomebrew, deployHomebrew); err != nil {
		return err
	}
	if err := checkValidateJob(validate); err != nil {
		return err
	}
	if err := checkBuildJob(build); err != nil {
		return err
	}
	if err := checkProductionBuildJob(productionBuild); err != nil {
		return err
	}
	if err := checkRecoveryPolicy(root); err != nil {
		return err
	}
	if err := checkVerifyReleaseSourceJob(verifyReleaseSource); err != nil {
		return err
	}
	signingIndex, publishSteps, err := checkPublishJob(publish)
	if err != nil {
		return err
	}
	if err := checkSigningSecretReachability(validate, build, productionBuild, verifyReleaseSource, publish, publishSteps, signingIndex); err != nil {
		return err
	}
	if err := checkRenderHomebrewJob(renderHomebrew); err != nil {
		return err
	}
	if err := checkVerifyHomebrewJob(verifyHomebrew); err != nil {
		return err
	}
	if err := checkDeployHomebrewJob(deployHomebrew); err != nil {
		return err
	}
	return nil
}

func checkTagTrigger(root *yaml.Node) error {
	on, ok := mappingValue(root, "on")
	if !ok || on.Kind != yaml.MappingNode {
		return errors.New("workflow must have an on mapping")
	}
	if err := requireOnlyMappingKeys(on, "push", "workflow_dispatch"); err != nil {
		return errors.New("on must contain only push and workflow_dispatch triggers")
	}
	push, ok := mappingValue(on, "push")
	if !ok || push.Kind != yaml.MappingNode {
		return errors.New("on.push must be a mapping")
	}
	if err := requireOnlyMappingKeys(push, "tags"); err != nil {
		return errors.New("on.push must contain only tags")
	}
	tags, ok := mappingValue(push, "tags")
	if !ok || tags.Kind != yaml.SequenceNode || len(tags.Content) != 1 || tags.Content[0].Value != "v[0-9]*" {
		return errors.New("on.push.tags must equal [v[0-9]*]")
	}
	dispatch, ok := mappingValue(on, "workflow_dispatch")
	if !ok || requireOnlyMappingKeys(dispatch, "inputs") != nil {
		return errors.New("workflow_dispatch must contain only the exact release_tag input")
	}
	inputs, ok := mappingValue(dispatch, "inputs")
	if !ok || requireOnlyMappingKeys(inputs, "release_tag") != nil {
		return errors.New("workflow_dispatch must contain only the exact release_tag input")
	}
	releaseTag, ok := mappingValue(inputs, "release_tag")
	if !ok || requireOnlyMappingKeys(releaseTag, "description", "required", "type") != nil || requireScalar(releaseTag, "required", "true") != nil || requireScalar(releaseTag, "type", "string") != nil {
		return errors.New("workflow_dispatch release_tag must be a required string")
	}
	return nil
}

func checkRecoveryPolicy(root *yaml.Node) error {
	if err := checkTagTrigger(root); err != nil {
		return err
	}
	env, ok := mappingValue(root, "env")
	if !ok || requireOnlyMappingKeys(env, "RELEASE_TAG") != nil || requireScalar(env, "RELEASE_TAG", "${{ github.event_name == 'workflow_dispatch' && inputs.release_tag || github.ref_name }}") != nil {
		return errors.New("workflow must bind RELEASE_TAG only to the push tag or required dispatch input")
	}
	jobs, ok := mappingValue(root, "jobs")
	if !ok {
		return errors.New("workflow must have jobs for recovery policy")
	}
	build, ok := mappingValue(jobs, "build")
	if !ok {
		return errors.New("workflow must have a build job for recovery policy")
	}
	buildEnv, ok := mappingValue(build, "env")
	if !ok || requireOnlyMappingKeys(buildEnv, "CC") != nil || requireScalar(buildEnv, "CC", "${{ matrix.cc }}") != nil {
		return errors.New("build job must bind CC from the audited matrix")
	}
	matrix := mustMappingPath(build, "strategy", "matrix")
	if matrix == nil {
		return errors.New("build matrix must contain exactly the six release targets")
	}
	if err := checkReleaseMatrix(matrix); err != nil {
		return err
	}
	productionBuild, ok := mappingValue(jobs, "build_production")
	if !ok || productionBuild.Kind != yaml.MappingNode {
		return errors.New("workflow must have an isolated production build job for recovery")
	}
	if containsScalarFragment(productionBuild, "GITHUB_SHA") || stepIndexWithRunFragment(mustJobSteps(productionBuild), "account_test.go") >= 0 {
		return errors.New("production build must not read the workflow SHA or recovery test overlay")
	}
	if containsScalarFragment(root, "GITHUB_REF_NAME") {
		return errors.New("release workflow must not derive production identity from GITHUB_REF_NAME")
	}
	if countScalarFragment(root, "GITHUB_SHA") != 1 {
		return errors.New("only the test overlay may read the audited workflow GITHUB_SHA")
	}
	steps, err := jobSteps(build)
	if err != nil {
		return errors.New("build job must contain recovery overlay gates")
	}
	if err := requireCanonicalBuildSteps(steps); err != nil {
		return err
	}
	applyIndex := stepIndexWithRunFragment(steps, `git show "${GITHUB_SHA}:internal/cli/account_test.go"`)
	preCommitIndex := stepIndexWithRunFragment(steps, "python -m pre_commit run --all-files")
	checkoutIndices := actionStepIndices(steps, canonicalCheckoutAction)
	if len(checkoutIndices) != 2 {
		return errors.New("build job must use exactly two canonical checkouts")
	}
	freshCheckoutIndex := checkoutIndices[1]
	if err := requireCanonicalCheckout(steps[freshCheckoutIndex], "fresh production checkout", checkoutWithRequirement{"fetch-depth", "0"}, checkoutWithRequirement{"persist-credentials", "false"}, checkoutWithRequirement{"ref", "${{ env.RELEASE_TAG }}"}, checkoutWithRequirement{"clean", "true"}); err != nil {
		return err
	}
	rebuildIndex := stepIndexWithRunFragment(steps, "bash scripts/build-staticlibs.sh --target '${{ matrix.rust_target }}'")
	diffIndex := stepIndexWithRunFragment(steps, "git diff --check")
	buildIndex := stepIndexWithRunFragment(steps, "go build -trimpath")
	packageIndex := stepIndexWithRunFragment(steps, "go run ./scripts/releaseassets package")
	if applyIndex < 0 || rebuildIndex <= freshCheckoutIndex || buildIndex <= rebuildIndex || packageIndex <= buildIndex {
		return errors.New("recovery must test the overlay, clean-checkout the tag, rebuild staticlib, then build and package")
	}
	if diffIndex >= 0 && (freshCheckoutIndex != len(steps)-6 || rebuildIndex != freshCheckoutIndex+1 || diffIndex != rebuildIndex+1 || buildIndex != diffIndex+1 || packageIndex != buildIndex+1) {
		return errors.New("fresh production checkout must begin the exact uninterrupted rebuild/build/package suffix")
	}
	if preCommitIndex >= 0 && freshCheckoutIndex <= preCommitIndex {
		return errors.New("recovery must finish pre-commit before the fresh production checkout")
	}
	if diffIndex >= 0 && (diffIndex <= rebuildIndex || buildIndex <= diffIndex) {
		return errors.New("recovery must diff-check the rebuilt tag inputs before production build")
	}
	if err := requireRecoveryOverlayStep(steps[applyIndex]); err != nil {
		return err
	}
	if err := requireOverlayQualitySequence(steps, applyIndex, freshCheckoutIndex); err != nil {
		return err
	}
	if err := requireProductionRebuildStep(steps[rebuildIndex]); err != nil {
		return err
	}
	if err := requireProductionBuildStep(steps[buildIndex]); err != nil {
		return err
	}
	if err := requireProductionPackageStep(steps[packageIndex]); err != nil {
		return err
	}
	return nil
}

func mustJobSteps(job *yaml.Node) []*yaml.Node {
	steps, err := jobSteps(job)
	if err != nil {
		return nil
	}
	return steps
}

func containsScalarFragment(node *yaml.Node, fragment string) bool {
	if node == nil {
		return false
	}
	if node.Kind == yaml.ScalarNode && strings.Contains(node.Value, fragment) {
		return true
	}
	for _, child := range node.Content {
		if containsScalarFragment(child, fragment) {
			return true
		}
	}
	return false
}

func countScalarFragment(node *yaml.Node, fragment string) int {
	if node == nil {
		return 0
	}
	count := 0
	if node.Kind == yaml.ScalarNode {
		count += strings.Count(node.Value, fragment)
	}
	for _, child := range node.Content {
		count += countScalarFragment(child, fragment)
	}
	return count
}

func stepIndexWithRunFragment(steps []*yaml.Node, fragment string) int {
	for index, step := range steps {
		if strings.Contains(requireRunValue(step), fragment) {
			return index
		}
	}
	return -1
}

func actionStepIndices(steps []*yaml.Node, action string) []int {
	var indices []int
	for index, step := range steps {
		uses, ok := mappingValue(step, "uses")
		if ok && uses.Kind == yaml.ScalarNode && uses.Value == action {
			indices = append(indices, index)
		}
	}
	return indices
}

func requireCanonicalBuildSteps(steps []*yaml.Node) error {
	if len(steps) != 21 {
		return errors.New("build job must contain exactly the 21 canonical steps")
	}
	if err := requireCanonicalCheckout(steps[0], "initial build checkout", checkoutWithRequirement{"fetch-depth", "0"}, checkoutWithRequirement{"persist-credentials", "false"}, checkoutWithRequirement{"ref", "${{ env.RELEASE_TAG }}"}); err != nil {
		return err
	}
	if err := requireExactActionStep(steps[1], "build Go setup", setupGoAction, map[string]string{"go-version": "1.26.3"}); err != nil {
		return err
	}
	for index, gate := range []struct {
		name      string
		command   string
		directory string
	}{
		{name: "Validate the exact immutable release source", command: `go run ./scripts/releaseassets validate --version "${RELEASE_TAG#v}"`},
		{name: "Install the native Rust target", command: "rustup target add '${{ matrix.rust_target }}'"},
		{name: "Check vendored Rust sources", command: "sh scripts/test-rust-vendor.sh"},
		{name: "Check Rust formatting from vendored sources", command: "cargo fmt --check", directory: "internal/download/ugoira_rs"},
		{name: "Lint vendored Rust sources", command: "cargo clippy --locked --offline --all-targets -- -D warnings", directory: "internal/download/ugoira_rs"},
	} {
		step := steps[index+2]
		var err error
		if gate.directory == "" {
			err = requireCanonicalNamedRunStep(step, gate.name, gate.command)
		} else {
			err = requireCanonicalNamedRunStepInDirectory(step, gate.name, gate.directory, gate.command)
		}
		if err != nil {
			return err
		}
	}
	if err := requireRecoveryOverlayStep(steps[7]); err != nil {
		return err
	}
	if err := requireScalar(steps[7], "name", "Apply the audited test-only recovery overlay"); err != nil {
		return errors.New("recovery overlay must keep its canonical name")
	}
	if err := requireOverlayQualitySequence(steps, 7, 15); err != nil {
		return err
	}
	if err := requireCanonicalCheckout(steps[15], "fresh production checkout", checkoutWithRequirement{"fetch-depth", "0"}, checkoutWithRequirement{"persist-credentials", "false"}, checkoutWithRequirement{"ref", "${{ env.RELEASE_TAG }}"}, checkoutWithRequirement{"clean", "true"}); err != nil {
		return err
	}
	if err := requireProductionRebuildStep(steps[16]); err != nil {
		return err
	}
	if err := requireScalar(steps[16], "name", "Rebuild the selected static library from the immutable tag"); err != nil {
		return errors.New("production staticlib rebuild must keep its canonical name")
	}
	if err := requireCanonicalNamedRunStep(steps[17], "Check the generated diff", "git diff --check"); err != nil {
		return err
	}
	if err := requireProductionBuildStep(steps[18]); err != nil {
		return err
	}
	if err := requireScalar(steps[18], "name", "Build the versioned native executable"); err != nil {
		return errors.New("production build must keep its canonical name")
	}
	if err := requireProductionPackageStep(steps[19]); err != nil {
		return err
	}
	if err := requireScalar(steps[19], "name", "Package the fixed-name platform asset"); err != nil {
		return errors.New("production package must keep its canonical name")
	}
	if err := requireExactActionStep(steps[20], "test-gate build artifact upload", uploadArtifactAction, map[string]string{
		"name":              "test-gate-${{ matrix.artifact }}",
		"path":              "dist/pixiv-cli_*",
		"if-no-files-found": "error",
		"retention-days":    "1",
	}); err != nil {
		return err
	}
	return nil
}

func requireCanonicalNamedRunStep(step *yaml.Node, name, command string) error {
	if err := requireCanonicalRunStep(step, name, command); err != nil {
		return err
	}
	if err := requireScalar(step, "name", name); err != nil {
		return fmt.Errorf("%s must keep its canonical name", name)
	}
	return nil
}

func requireCanonicalNamedRunStepInDirectory(step *yaml.Node, name, directory, command string) error {
	if err := requireOnlyMappingKeys(step, "name", "shell", "working-directory", "run"); err != nil || requireScalar(step, "name", name) != nil || requireScalar(step, "shell", "bash") != nil || requireScalar(step, "working-directory", directory) != nil || requireScalar(step, "run", command) != nil {
		return fmt.Errorf("%s must be the exact canonical step in %s", name, directory)
	}
	return nil
}

func requireRecoveryOverlayStep(step *yaml.Node) error {
	const commands = `
set -euo pipefail
test -z "$(git diff --name-only)"
test -z "$(git diff --cached --name-only)"
git show "${GITHUB_SHA}:internal/cli/account_test.go" > internal/cli/account_test.go
test "$(git diff --name-only)" = internal/cli/account_test.go
test -z "$(git diff --cached --name-only)"`
	if err := requireCanonicalConditionalRunStep(step, "recovery overlay", "github.event_name == 'workflow_dispatch'", commands); err != nil {
		return errors.New("recovery overlay must modify only internal/cli/account_test.go with the exact commands")
	}
	return nil
}

func requireOverlayQualitySequence(steps []*yaml.Node, overlayIndex, freshCheckoutIndex int) error {
	expected := []struct {
		name    string
		command string
	}{
		{name: "Test Go sources", command: "go test ./..."},
		{name: "Test Go sources with the race detector", command: "go test -race ./..."},
		{name: "Vet Go sources", command: "go vet ./..."},
		{name: "Audit bundled licenses", command: "go run ./scripts/licensebundle --check"},
		{name: "Test release packages", command: "sh scripts/test-package-release.sh"},
		{name: "Install the pinned pre-commit version", command: "python -m pip install --disable-pip-version-check pre-commit==4.6.0"},
		{name: "Run pre-commit checks", command: "python -m pre_commit run --all-files"},
	}
	if freshCheckoutIndex != overlayIndex+len(expected)+1 {
		return errors.New("overlay quality sequence must contain exactly the canonical gates before fresh checkout")
	}
	for offset, gate := range expected {
		step := steps[overlayIndex+offset+1]
		if err := requireCanonicalRunStep(step, "overlay quality gate "+gate.name, gate.command); err != nil {
			return errors.New("overlay quality sequence must preserve exact canonical gate commands and order")
		}
		if err := requireScalar(step, "name", gate.name); err != nil {
			return errors.New("overlay quality sequence must preserve exact canonical gate names and order")
		}
	}
	return nil
}

func requireProductionRebuildStep(step *yaml.Node) error {
	const commands = `
set -euo pipefail
test "$(git rev-parse HEAD)" = "$(git rev-parse "$RELEASE_TAG^{commit}")"
test -z "$(git status --porcelain --untracked-files=all)"
test -z "$(git clean -ndx)"
bash scripts/build-staticlibs.sh --target '${{ matrix.rust_target }}'
git restore --source="$RELEASE_TAG^{commit}" -- internal/download/ugoira_rs/staticlib/manifest.json
git diff --exit-code
test -z "$(git status --porcelain --untracked-files=all)"
test -z "$(git clean -ndx)"`
	if err := requireCanonicalRunStep(step, "production staticlib rebuild", commands); err != nil {
		return errors.New("production staticlib rebuild must use the exact clean tag command sequence")
	}
	return nil
}

func requireProductionBuildStep(step *yaml.Node) error {
	const commands = `
set -eu
mkdir -p dist
output='dist/pixiv'
if [ '${{ matrix.goos }}' = windows ]; then
output='dist/pixiv.exe'
fi
go build -trimpath -buildvcs=false \
-ldflags "-X github.com/FlanChanXwO/pixiv-cli/internal/buildinfo.Version=${RELEASE_TAG} -X github.com/FlanChanXwO/pixiv-cli/internal/buildinfo.Commit=$(git rev-parse HEAD) -X github.com/FlanChanXwO/pixiv-cli/internal/buildinfo.BuildDate=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
-o "$output" ./cmd/pixiv`
	if err := requireCanonicalRunStep(step, "production versioned binary build", commands); err != nil {
		return errors.New("production build must use the exact tag-bound metadata and output command sequence")
	}
	return nil
}

func requireProductionPackageStep(step *yaml.Node) error {
	const commands = `
go run ./scripts/releaseassets package \
--repo-root . \
--version "${RELEASE_TAG#v}" \
--target '${{ matrix.goos }}/${{ matrix.goarch }}' \
--binary "dist/pixiv${{ matrix.goos == 'windows' && '.exe' || '' }}" \
--output-dir dist`
	if err := requireCanonicalRunStep(step, "production asset package", commands); err != nil {
		return errors.New("production package must use the exact tag, target, binary and output command sequence")
	}
	return nil
}

func requireCanonicalConditionalRunStep(step *yaml.Node, context, condition, canonical string) error {
	if err := requireOnlyMappingKeys(step, "name", "if", "shell", "run"); err != nil {
		return fmt.Errorf("%s must be the canonical conditional bash step", context)
	}
	if err := requireScalar(step, "if", condition); err != nil {
		return fmt.Errorf("%s must use only the approved condition", context)
	}
	if err := requireScalar(step, "shell", "bash"); err != nil || !equalCommands(splitCommands(requireRunValue(step)), splitCommands(canonical)) {
		return fmt.Errorf("%s must use the exact command sequence", context)
	}
	return nil
}

func mustMappingPath(root *yaml.Node, keys ...string) *yaml.Node {
	current := root
	for _, key := range keys {
		var ok bool
		current, ok = mappingValue(current, key)
		if !ok {
			return nil
		}
	}
	return current
}

func checkGlobalPermissions(root *yaml.Node) error {
	permissions, ok := mappingValue(root, "permissions")
	if !ok || permissions.Kind != yaml.MappingNode || len(permissions.Content) != 0 {
		return errors.New("global permissions must be an empty mapping")
	}
	return nil
}

func checkActionReferences(root *yaml.Node) error {
	var references []string
	invalidReference := false
	collectActionReferences(root, &references, &invalidReference)
	if len(references) == 0 {
		return errors.New("workflow must use at least one action")
	}
	if invalidReference {
		return errors.New("every action uses reference must be an owner/repo full 40-character lowercase SHA")
	}
	for _, reference := range references {
		if !actionReferencePattern.MatchString(reference) {
			return errors.New("every action uses reference must be an owner/repo full 40-character lowercase SHA")
		}
	}
	return nil
}

func collectActionReferences(node *yaml.Node, references *[]string, invalidReference *bool) {
	if node == nil {
		return
	}
	if node.Kind == yaml.MappingNode {
		for index := 0; index+1 < len(node.Content); index += 2 {
			key, value := node.Content[index], node.Content[index+1]
			if key.Value == "uses" {
				if value.Kind != yaml.ScalarNode {
					*invalidReference = true
				} else {
					*references = append(*references, value.Value)
				}
			}
			collectActionReferences(value, references, invalidReference)
		}
		return
	}
	for _, child := range node.Content {
		collectActionReferences(child, references, invalidReference)
	}
}

func checkValidateJob(job *yaml.Node) error {
	if err := requireRequiredJobExecution(job, "validate job"); err != nil {
		return err
	}
	if err := requireNoEnvironment(job, "validate job"); err != nil {
		return err
	}
	if err := requireOnlyMappingKeys(job, "name", "runs-on", "permissions", "steps"); err != nil {
		return fmt.Errorf("validate job: %w", err)
	}
	if err := requireScalar(job, "runs-on", "ubuntu-24.04"); err != nil {
		return fmt.Errorf("validate job: %w", err)
	}
	if _, exists := mappingValue(job, "needs"); exists {
		return errors.New("validate job must not depend on another job")
	}
	if err := requireContentsPermission(job, "read"); err != nil {
		return fmt.Errorf("validate job: %w", err)
	}
	steps, err := jobSteps(job)
	if err != nil || len(steps) < 3 {
		return errors.New("validate job must contain the audited workflow checkout and release tag gates")
	}
	if err := requireCanonicalCheckout(steps[0], "validate job", checkoutWithRequirement{"fetch-depth", "0"}, checkoutWithRequirement{"persist-credentials", "false"}, checkoutWithRequirement{"ref", "${{ github.sha }}"}); err != nil {
		return err
	}
	validateStep, ok := rootStepWithRunFragment(job, "go run ./scripts/releaseassets validate --version \"${RELEASE_TAG#v}\"")
	if !ok {
		return errors.New("validate job must validate RELEASE_TAG with releaseassets")
	}
	if err := requireRunFragments(validateStep, "validate release tag step", "test \"$GITHUB_REF\" = \"refs/heads/$DEFAULT_BRANCH\"", "git show-ref --verify --quiet \"refs/tags/$RELEASE_TAG\"", "git merge-base --is-ancestor \"$tag_commit\" \"origin/$DEFAULT_BRANCH\"", "gh api --include \"repos/$GITHUB_REPOSITORY/releases/tags/$RELEASE_TAG\"", "HTTP/[0-9.]+ 404"); err != nil {
		return err
	}
	if !hasCommand(job, "", "sh scripts/test-release-workflow.sh") {
		return errors.New("validate job must run the release workflow policy")
	}
	return nil
}

func checkBuildJob(job *yaml.Node) error {
	if err := requireRequiredJobExecution(job, "build job"); err != nil {
		return err
	}
	if err := requireNoEnvironment(job, "build job"); err != nil {
		return err
	}
	if err := requireOnlyMappingKeys(job, "name", "needs", "runs-on", "permissions", "env", "strategy", "steps"); err != nil {
		return fmt.Errorf("build job: %w", err)
	}
	if err := requireScalar(job, "needs", "validate"); err != nil {
		return fmt.Errorf("build job: %w", err)
	}
	if err := requireScalar(job, "runs-on", "${{ matrix.runner }}"); err != nil {
		return fmt.Errorf("build job: %w", err)
	}
	if err := requireContentsPermission(job, "read"); err != nil {
		return fmt.Errorf("build job: %w", err)
	}
	steps, err := jobSteps(job)
	if err != nil || len(steps) == 0 {
		return errors.New("build job must contain steps")
	}
	if err := requireCanonicalCheckout(steps[0], "build job", checkoutWithRequirement{"fetch-depth", "0"}, checkoutWithRequirement{"persist-credentials", "false"}, checkoutWithRequirement{"ref", "${{ env.RELEASE_TAG }}"}); err != nil {
		return err
	}
	strategy, ok := mappingValue(job, "strategy")
	if !ok || strategy.Kind != yaml.MappingNode {
		return errors.New("build job must have a matrix strategy")
	}
	if err := requireScalar(strategy, "fail-fast", "false"); err != nil {
		return fmt.Errorf("build strategy: %w", err)
	}
	matrix, ok := mappingValue(strategy, "matrix")
	if !ok || matrix.Kind != yaml.MappingNode {
		return errors.New("build job must have a matrix")
	}
	if err := checkReleaseMatrix(matrix); err != nil {
		return err
	}

	for _, gate := range []struct {
		workingDirectory string
		command          string
	}{
		{command: "go run ./scripts/releaseassets validate --version \"${RELEASE_TAG#v}\""},
		{command: "sh scripts/test-rust-vendor.sh"},
		{workingDirectory: "internal/download/ugoira_rs", command: "cargo fmt --check"},
		{workingDirectory: "internal/download/ugoira_rs", command: "cargo clippy --locked --offline --all-targets -- -D warnings"},
		{command: "go test ./..."},
		{command: "go test -race ./..."},
		{command: "go vet ./..."},
		{command: "go run ./scripts/licensebundle --check"},
		{command: "sh scripts/test-package-release.sh"},
		{command: "python -m pip install --disable-pip-version-check pre-commit==4.6.0"},
		{command: "python -m pre_commit run --all-files"},
		{command: "git diff --check"},
	} {
		if err := requireIndependentQualityGate(job, gate.workingDirectory, gate.command); err != nil {
			return err
		}
	}
	if !hasCommand(job, "", "bash scripts/build-staticlibs.sh --target '${{ matrix.rust_target }}'") {
		return errors.New("build must build the selected native Rust static library")
	}
	packageStep, ok := rootStepWithRunFragment(job, "go run ./scripts/releaseassets package")
	if !ok {
		return errors.New("build must package the selected platform asset")
	}
	if err := requireRunFragments(packageStep, "build packaging step", "--repo-root .", "--target '${{ matrix.goos }}/${{ matrix.goarch }}'", "--output-dir dist"); err != nil {
		return err
	}
	return nil
}

// checkProductionBuildJob 将最终资产放入独立 runner：它只能读取 immutable tag，不能承接
// recovery 测试进程对环境变量、PATH 或临时目录的跨 step 副作用。
func checkProductionBuildJob(job *yaml.Node) error {
	if err := requireRequiredJobExecution(job, "build_production job"); err != nil {
		return err
	}
	if err := requireNoEnvironment(job, "build_production job"); err != nil {
		return err
	}
	if err := requireOnlyMappingKeys(job, "name", "needs", "runs-on", "permissions", "env", "strategy", "steps"); err != nil {
		return fmt.Errorf("build_production job: %w", err)
	}
	if err := requireScalar(job, "name", "Build production ${{ matrix.goos }}/${{ matrix.goarch }}"); err != nil {
		return fmt.Errorf("build_production job: %w", err)
	}
	if err := requireScalar(job, "needs", "build"); err != nil {
		return fmt.Errorf("build_production job: %w", err)
	}
	if err := requireScalar(job, "runs-on", "${{ matrix.runner }}"); err != nil {
		return fmt.Errorf("build_production job: %w", err)
	}
	if err := requireContentsPermission(job, "read"); err != nil {
		return fmt.Errorf("build_production job: %w", err)
	}
	env, ok := mappingValue(job, "env")
	if !ok || requireOnlyMappingKeys(env, "CC") != nil || requireScalar(env, "CC", "${{ matrix.cc }}") != nil {
		return errors.New("build_production job must bind CC from the audited matrix")
	}
	strategy, ok := mappingValue(job, "strategy")
	if !ok || requireOnlyMappingKeys(strategy, "fail-fast", "matrix") != nil || requireScalar(strategy, "fail-fast", "false") != nil {
		return errors.New("build_production strategy must contain only fail-fast: false and the release matrix")
	}
	matrix := mustMappingPath(job, "strategy", "matrix")
	if matrix == nil || checkReleaseMatrix(matrix) != nil {
		return errors.New("build_production matrix must contain exactly the six release targets")
	}
	steps, err := jobSteps(job)
	if err != nil || len(steps) != 9 {
		return errors.New("build_production job must contain exactly the immutable production steps")
	}
	if err := requireCanonicalCheckout(steps[0], "build_production job", checkoutWithRequirement{"clean", "true"}, checkoutWithRequirement{"fetch-depth", "0"}, checkoutWithRequirement{"persist-credentials", "false"}, checkoutWithRequirement{"ref", "${{ env.RELEASE_TAG }}"}); err != nil {
		return err
	}
	if err := requireExactActionStep(steps[1], "build_production Go setup", setupGoAction, map[string]string{"go-version": "1.26.3", "cache": "false"}); err != nil {
		return err
	}
	if err := requireCanonicalNamedRunStep(steps[2], "Validate the exact immutable production source", `go run ./scripts/releaseassets validate --version "${RELEASE_TAG#v}"`); err != nil {
		return err
	}
	if err := requireCanonicalNamedRunStep(steps[3], "Install the native Rust target", "rustup target add '${{ matrix.rust_target }}'"); err != nil {
		return err
	}
	if err := requireProductionRebuildStep(steps[4]); err != nil || requireScalar(steps[4], "name", "Rebuild the selected static library from the immutable tag") != nil {
		return errors.New("build_production job must rebuild the selected static library from the immutable tag")
	}
	if err := requireCanonicalNamedRunStep(steps[5], "Check the generated diff", "git diff --check"); err != nil {
		return err
	}
	if err := requireProductionBuildStep(steps[6]); err != nil || requireScalar(steps[6], "name", "Build the versioned native executable") != nil {
		return errors.New("build_production job must build the tag-bound executable")
	}
	if err := requireProductionPackageStep(steps[7]); err != nil || requireScalar(steps[7], "name", "Package the fixed-name platform asset") != nil {
		return errors.New("build_production job must package the tag-bound executable")
	}
	return requireExactActionStep(steps[8], "verified release build artifact upload", uploadArtifactAction, map[string]string{
		"name":              "verified-release-${{ matrix.artifact }}",
		"path":              "dist/pixiv-cli_*",
		"if-no-files-found": "error",
		"retention-days":    "1",
	})
}

// rejectAmbiguousYAML 在策略读取字段前拒绝 GitHub Actions 与 yaml.v3 可能采用不同语义的
// YAML 构造。mappingValue 只会返回第一个同名字段，故必须先消除重复键和 merge 的歧义。
func rejectAmbiguousYAML(node *yaml.Node) error {
	if node == nil {
		return errors.New("workflow must not contain nil YAML nodes")
	}
	if node.Kind == yaml.AliasNode {
		return errors.New("workflow must not use YAML aliases")
	}
	if node.Kind == yaml.MappingNode {
		if len(node.Content)%2 != 0 {
			return errors.New("workflow mappings must contain key-value pairs")
		}
		keys := make(map[string]struct{}, len(node.Content)/2)
		for index := 0; index < len(node.Content); index += 2 {
			key, value := node.Content[index], node.Content[index+1]
			if key.Kind != yaml.ScalarNode {
				return errors.New("workflow mapping keys must be scalars")
			}
			if key.Value == "<<" {
				return errors.New("workflow must not use YAML merge keys")
			}
			if _, duplicate := keys[key.Value]; duplicate {
				return fmt.Errorf("workflow must not contain duplicate mapping key %q", key.Value)
			}
			keys[key.Value] = struct{}{}
			if err := rejectAmbiguousYAML(key); err != nil {
				return err
			}
			if err := rejectAmbiguousYAML(value); err != nil {
				return err
			}
		}
		return nil
	}
	for _, child := range node.Content {
		if err := rejectAmbiguousYAML(child); err != nil {
			return err
		}
	}
	return nil
}

func requireNoWorkflowExecutionOverrides(root *yaml.Node) error {
	if _, exists := mappingValue(root, "defaults"); exists {
		return errors.New("workflow root must not declare defaults")
	}
	return nil
}

func requireNoEnvironment(job *yaml.Node, jobName string) error {
	if _, exists := mappingValue(job, "environment"); exists {
		return fmt.Errorf("%s must not declare an environment", jobName)
	}
	return nil
}

func requireRequiredJobExecution(job *yaml.Node, jobName string) error {
	for _, key := range []string{"if", "continue-on-error"} {
		if _, exists := mappingValue(job, key); exists {
			return fmt.Errorf("%s must not define if or continue-on-error", jobName)
		}
	}
	if _, exists := mappingValue(job, "defaults"); exists {
		return fmt.Errorf("%s must not declare defaults", jobName)
	}
	return nil
}

// requireIndependentQualityGate 将每项质量命令约束为唯一、单命令的 bash step。相比从
// 多行 shell 文本中猜测控制流，这样可以证明 gate 不会藏在 if、循环或 `|| true` 之中。
func requireIndependentQualityGate(job *yaml.Node, directory, command string) error {
	steps, err := jobSteps(job)
	if err != nil {
		return fmt.Errorf("build must run %s", command)
	}
	var matches []*yaml.Node
	for _, step := range steps {
		if requireRunValue(step) == command {
			matches = append(matches, step)
		}
	}
	if len(matches) != 1 {
		return fmt.Errorf("build must run %s in an independent quality gate step", command)
	}
	step := matches[0]
	for _, key := range []string{"continue-on-error", "if"} {
		if _, exists := mappingValue(step, key); exists {
			return fmt.Errorf("build quality gate %s must not define continue-on-error or if", command)
		}
	}
	keys := []string{"name", "shell", "run"}
	if directory != "" {
		keys = append(keys, "working-directory")
	}
	if err := requireOnlyMappingKeys(step, keys...); err != nil {
		return fmt.Errorf("build quality gate %s must be an independent direct bash step", command)
	}
	if err := requireScalar(step, "shell", "bash"); err != nil {
		return fmt.Errorf("build quality gate %s must use shell bash", command)
	}
	if directory == "" {
		if _, exists := mappingValue(step, "working-directory"); exists {
			return fmt.Errorf("build quality gate %s must run from the repository root", command)
		}
		return nil
	}
	if err := requireScalar(step, "working-directory", directory); err != nil {
		return fmt.Errorf("build quality gate %s must run from %s", command, directory)
	}
	return nil
}

func checkReleaseMatrix(matrix *yaml.Node) error {
	if err := requireOnlyMappingKeys(matrix, "include"); err != nil {
		return errors.New("build matrix must contain only the six release targets")
	}
	include, ok := mappingValue(matrix, "include")
	if !ok || include.Kind != yaml.SequenceNode || len(include.Content) != len(releaseMatrixTargets) {
		return errors.New("build matrix must contain exactly the six release targets")
	}
	seen := make(map[string]struct{}, len(include.Content))
	for _, entry := range include.Content {
		if entry.Kind != yaml.MappingNode {
			return errors.New("build matrix must contain exactly the six release targets")
		}
		if err := requireOnlyMappingKeys(entry, "runner", "goos", "goarch", "rust_target", "artifact", "cc"); err != nil {
			return errors.New("build matrix entries must contain only the canonical release target fields")
		}
		fields := make([]string, 0, 6)
		for _, key := range []string{"runner", "goos", "goarch", "rust_target", "artifact", "cc"} {
			value, ok := mappingValue(entry, key)
			if !ok || value.Kind != yaml.ScalarNode {
				return errors.New("build matrix must contain exactly the six release targets")
			}
			fields = append(fields, value.Value)
		}
		identity := strings.Join(fields, "|")
		if _, expected := releaseMatrixTargets[identity]; !expected {
			return errors.New("build matrix must contain exactly the six release targets")
		}
		if _, duplicate := seen[identity]; duplicate {
			return errors.New("build matrix must contain exactly the six release targets")
		}
		seen[identity] = struct{}{}
	}
	return nil
}

func checkVerifyReleaseSourceJob(job *yaml.Node) error {
	if err := requireRequiredJobExecution(job, "verify_release_source job"); err != nil {
		return err
	}
	if err := requireNoEnvironment(job, "verify_release_source job"); err != nil {
		return err
	}
	if err := requireOnlyMappingKeys(job, "name", "needs", "runs-on", "permissions", "steps"); err != nil {
		return fmt.Errorf("verify_release_source job: %w", err)
	}
	if err := requireScalar(job, "needs", "build_production"); err != nil {
		return fmt.Errorf("verify_release_source job: %w", err)
	}
	if err := requireScalar(job, "runs-on", "ubuntu-24.04"); err != nil {
		return fmt.Errorf("verify_release_source job: %w", err)
	}
	if err := requireContentsPermission(job, "read"); err != nil {
		return fmt.Errorf("verify_release_source job: %w", err)
	}
	steps, err := jobSteps(job)
	if err != nil {
		return fmt.Errorf("verify_release_source job: %w", err)
	}
	if len(steps) != 2 {
		return errors.New("verify_release_source job must contain only the canonical checkout and ancestry gate steps")
	}
	if err := requireCanonicalCheckout(steps[0], "verify_release_source job", checkoutWithRequirement{"fetch-depth", "0"}, checkoutWithRequirement{"persist-credentials", "false"}, checkoutWithRequirement{"ref", "${{ env.RELEASE_TAG }}"}); err != nil {
		return err
	}
	ancestryGate := steps[1]
	if !hasStepCommand(ancestryGate, "git merge-base --is-ancestor HEAD \"origin/$DEFAULT_BRANCH\"") {
		return errors.New("verify_release_source job must contain a default-branch ancestry gate")
	}
	if err := requireUnconditionalAncestryGate(ancestryGate); err != nil {
		return err
	}
	if !hasStepCommand(ancestryGate, "git show-ref --verify --quiet \"refs/remotes/origin/$DEFAULT_BRANCH\"") {
		return errors.New("verify_release_source default-branch ancestry gate must verify origin/$DEFAULT_BRANCH")
	}
	env, ok := mappingValue(ancestryGate, "env")
	if !ok || requireOnlyMappingKeys(env, "DEFAULT_BRANCH") != nil || requireScalar(env, "DEFAULT_BRANCH", "${{ github.event.repository.default_branch }}") != nil {
		return errors.New("verify_release_source default-branch ancestry gate must derive DEFAULT_BRANCH from the release repository")
	}
	return nil
}

type checkoutWithRequirement struct {
	key   string
	value string
}

// requireCanonicalCheckout 将 source checkout 绑定到审计过的 action SHA 和精确的 with 字段，
// 防止 ref、repository、path 或额外选项让校验、构建、签名各自读取不同提交。
func requireCanonicalCheckout(step *yaml.Node, jobName string, requirements ...checkoutWithRequirement) error {
	if err := requireOnlyMappingKeys(step, "uses", "with"); err != nil {
		return fmt.Errorf("%s must use the canonical checkout", jobName)
	}
	if err := requireScalar(step, "uses", canonicalCheckoutAction); err != nil {
		return fmt.Errorf("%s must use the canonical checkout", jobName)
	}
	with, ok := mappingValue(step, "with")
	if !ok {
		return fmt.Errorf("%s must use the canonical checkout", jobName)
	}
	keys := make([]string, 0, len(requirements))
	for _, requirement := range requirements {
		keys = append(keys, requirement.key)
	}
	if err := requireOnlyMappingKeys(with, keys...); err != nil {
		return fmt.Errorf("%s must use the canonical checkout", jobName)
	}
	for _, requirement := range requirements {
		if err := requireScalar(with, requirement.key, requirement.value); err != nil {
			return fmt.Errorf("%s must use the canonical checkout", jobName)
		}
	}
	return nil
}

func requireVerifiedProductionDownloads(steps []*yaml.Node) error {
	names := []string{
		"verified-release-darwin-amd64",
		"verified-release-darwin-arm64",
		"verified-release-linux-amd64",
		"verified-release-linux-arm64",
		"verified-release-windows-amd64",
		"verified-release-windows-arm64",
	}
	indices := actionStepIndices(steps, downloadArtifactAction)
	if len(indices) != len(names) || containsScalarFragment(&yaml.Node{Kind: yaml.SequenceNode, Content: steps}, "test-gate-") {
		return errors.New("publish job must download exactly the six verified production artifacts")
	}
	for index, name := range names {
		if indices[index] != index+2 {
			return errors.New("publish verified production artifact downloads must be consecutive and ordered")
		}
		if err := requireExactActionStep(steps[index+2], "verified production asset download", downloadArtifactAction, map[string]string{
			"name": name,
			"path": "dist",
		}); err != nil {
			return errors.New("publish verified production artifact downloads must use exact names and dist path")
		}
	}
	return nil
}

func checkPublishJob(job *yaml.Node) (int, []*yaml.Node, error) {
	if err := requireRequiredJobExecution(job, "publish job"); err != nil {
		return 0, nil, err
	}
	if err := requireOnlyMappingKeys(job, "name", "needs", "runs-on", "environment", "permissions", "steps"); err != nil {
		return 0, nil, fmt.Errorf("publish job: %w", err)
	}
	if err := requireScalar(job, "needs", "verify_release_source"); err != nil {
		return 0, nil, fmt.Errorf("publish job: %w", err)
	}
	if err := requireScalar(job, "runs-on", "ubuntu-24.04"); err != nil {
		return 0, nil, fmt.Errorf("publish job: %w", err)
	}
	if err := requireScalar(job, "environment", "release"); err != nil {
		return 0, nil, errors.New("publish environment must be release")
	}
	if err := requireContentsPermission(job, "write"); err != nil {
		return 0, nil, fmt.Errorf("publish job: %w", err)
	}
	steps, err := jobSteps(job)
	if err != nil {
		return 0, nil, fmt.Errorf("publish job: %w", err)
	}
	if err := requireVerifiedProductionDownloads(steps); err != nil {
		return 0, nil, err
	}
	if err := requireCanonicalCheckout(steps[0], "publish job", checkoutWithRequirement{"persist-credentials", "false"}, checkoutWithRequirement{"ref", "${{ env.RELEASE_TAG }}"}); err != nil {
		return 0, nil, err
	}
	signingIndex, signingStep := signingStepIndex(steps)
	if signingIndex < 0 {
		return 0, nil, errors.New("publish job must contain a signing-secret step")
	}
	if err := checkSigningStep(signingStep); err != nil {
		return 0, nil, err
	}
	for _, step := range steps {
		run, hasRun := mappingValue(step, "run")
		if hasRun && strings.Contains(run.Value, "${RELEASE_TAG#v}") && strings.Contains(run.Value, "*-*") {
			return 0, nil, errors.New("publish job must not classify prereleases with a hyphen shell pattern")
		}
	}
	releaseStep, ok := rootStepWithRunFragment(job, "gh release create \"$RELEASE_TAG\"")
	if !ok {
		return 0, nil, errors.New("publish job must create and publish the verified GitHub Release")
	}
	if !hasShellArgument(releaseStep, "--draft") {
		return 0, nil, errors.New("release publishing step must contain --draft")
	}
	if !hasShellArgument(releaseStep, "--verify-tag") {
		return 0, nil, errors.New("release publishing step must contain --verify-tag")
	}
	if err := checkReleaseChannelBinding(releaseStep); err != nil {
		return 0, nil, err
	}
	if err := requireRunFragments(releaseStep, "release publishing step", "${prerelease[@]}", "release/checksums.json", "gh release view \"$RELEASE_TAG\"", "gh release edit \"$RELEASE_TAG\" --draft=false"); err != nil {
		return 0, nil, err
	}
	if err := requireCanonicalReleasePublicationSuffix(releaseStep); err != nil {
		return 0, nil, err
	}
	releaseIndex := -1
	for index, step := range steps {
		if step == releaseStep {
			releaseIndex = index
			break
		}
	}
	if releaseIndex < 0 || releaseIndex != len(steps)-2 {
		return 0, nil, errors.New("publish job must upload the verified release/checksums.txt after publishing")
	}
	if err := requireExactActionStep(steps[len(steps)-1], "published checksums artifact", uploadArtifactAction, map[string]string{
		"name":              "verified-release-checksums",
		"path":              "release/checksums.txt",
		"if-no-files-found": "error",
		"retention-days":    "1",
	}); err != nil {
		return 0, nil, errors.New("publish job must upload the verified release/checksums.txt after publishing")
	}
	return signingIndex, steps, nil
}

func requireCanonicalReleasePublicationSuffix(step *yaml.Node) error {
	const canonical = `
gh release create "$RELEASE_TAG" \
--draft \
--verify-tag \
--title "$RELEASE_TAG" \
--notes-file release/release-notes.md \
"${prerelease[@]}" \
release/pixiv-cli_*.tar.gz \
release/pixiv-cli_*.zip \
release/checksums.txt \
release/checksums.json
expected=$(find release -maxdepth 1 -type f ! -name release-notes.md -exec basename {} \; | LC_ALL=C sort)
actual=$(gh release view "$RELEASE_TAG" --json assets --jq '.assets[].name' | LC_ALL=C sort)
if [ "$actual" != "$expected" ]; then
printf '%s\n' 'draft release assets differ from the verified local set' >&2
printf '%s\n' "expected:$expected" >&2
printf '%s\n' "actual:$actual" >&2
exit 1
fi
gh release edit "$RELEASE_TAG" --draft=false`
	commands := splitCommands(requireRunValue(step))
	start := -1
	for index, command := range commands {
		if command == `gh release create "$RELEASE_TAG" \` {
			start = index
			break
		}
	}
	if start < 0 || !equalCommands(commands[start:], splitCommands(canonical)) {
		return errors.New("release publishing step must preserve the verified asset set before exporting checksums")
	}
	return nil
}

func requireExactActionStep(step *yaml.Node, context, action string, withValues map[string]string) error {
	if err := requireOnlyMappingKeys(step, "uses", "with"); err != nil {
		return fmt.Errorf("%s must be the exact pinned action step", context)
	}
	if err := requireScalar(step, "uses", action); err != nil {
		return fmt.Errorf("%s must be the exact pinned action step", context)
	}
	with, ok := mappingValue(step, "with")
	if !ok || with.Kind != yaml.MappingNode || len(with.Content) != len(withValues)*2 {
		return fmt.Errorf("%s must be the exact pinned action step", context)
	}
	for key, value := range withValues {
		if err := requireScalar(with, key, value); err != nil {
			return fmt.Errorf("%s must be the exact pinned action step", context)
		}
	}
	return nil
}

func requireCanonicalRunStep(step *yaml.Node, context, canonical string) error {
	if err := requireOnlyMappingKeys(step, "name", "shell", "run"); err != nil {
		return fmt.Errorf("%s must be the canonical direct bash step", context)
	}
	if err := requireScalar(step, "shell", "bash"); err != nil {
		return fmt.Errorf("%s must be the canonical direct bash step", context)
	}
	if !equalCommands(splitCommands(requireRunValue(step)), splitCommands(canonical)) {
		return fmt.Errorf("%s must use the required direct command sequence", context)
	}
	return nil
}

func checkRenderHomebrewJob(job *yaml.Node) error {
	const renderCommands = `
set -eu
version="${RELEASE_TAG#v}"
case "$(go run ./scripts/releaseassets channel --version "$version")" in
stable)
formula_name=pixiv-cli
;;
prerelease)
formula_name=pixiv-cli-beta
;;
*)
printf '%s\n' 'unexpected release classification' >&2
exit 1
;;
esac
mkdir -p staging-formula
go run ./scripts/homebrewformula render \
--formula "$formula_name" \
--version "$version" \
--checksums verified-release/checksums.txt \
--output "staging-formula/$formula_name.rb"
printf '%s\n' "$formula_name" > staging-formula/formula-name`

	if err := requireRequiredJobExecution(job, "render_homebrew_formula job"); err != nil {
		return err
	}
	if err := requireNoEnvironment(job, "render_homebrew_formula job"); err != nil {
		return err
	}
	if err := requireOnlyMappingKeys(job, "name", "needs", "runs-on", "permissions", "steps"); err != nil {
		return fmt.Errorf("render_homebrew_formula job: %w", err)
	}
	if err := requireScalar(job, "needs", "publish"); err != nil {
		return fmt.Errorf("render_homebrew_formula job: %w", err)
	}
	if err := requireScalar(job, "runs-on", "ubuntu-24.04"); err != nil {
		return fmt.Errorf("render_homebrew_formula job: %w", err)
	}
	if err := requireContentsPermission(job, "read"); err != nil {
		return fmt.Errorf("render_homebrew_formula job: %w", err)
	}
	steps, err := jobSteps(job)
	if err != nil || len(steps) != 5 {
		return errors.New("render_homebrew_formula job must contain only the canonical provenance and render steps")
	}
	if err := requireCanonicalCheckout(steps[0], "render_homebrew_formula job", checkoutWithRequirement{"persist-credentials", "false"}, checkoutWithRequirement{"ref", "${{ env.RELEASE_TAG }}"}); err != nil {
		return err
	}
	if err := requireExactActionStep(steps[1], "render_homebrew_formula Go setup", setupGoAction, map[string]string{"go-version": "1.26.3"}); err != nil {
		return err
	}
	if err := requireExactActionStep(steps[2], "verified checksums download", downloadArtifactAction, map[string]string{
		"name": "verified-release-checksums",
		"path": "verified-release",
	}); err != nil {
		return err
	}
	if err := requireCanonicalRunStep(steps[3], "Homebrew formula render step", renderCommands); err != nil {
		return err
	}
	if err := requireExactActionStep(steps[4], "staging formula artifact", uploadArtifactAction, map[string]string{
		"name":              "staging-homebrew-formula",
		"path":              "staging-formula",
		"if-no-files-found": "error",
		"retention-days":    "1",
	}); err != nil {
		return err
	}
	return nil
}

func checkVerifyHomebrewJob(job *yaml.Node) error {
	const verifyCommands = `
set -euo pipefail
if [ '${{ matrix.os }}' = linux ]; then
eval "$(/home/linuxbrew/.linuxbrew/bin/brew shellenv)"
fi
formula_name=$(cat staging-formula/formula-name)
case "$formula_name" in
pixiv-cli|pixiv-cli-beta) ;;
*) exit 1 ;;
esac
test "$(find staging-formula -maxdepth 1 -type f -print | LC_ALL=C sort)" = "$(printf '%s\n%s\n' staging-formula/formula-name "staging-formula/$formula_name.rb" | LC_ALL=C sort)"
staging_tap=pixiv-cli-release/staging
tap_dir="$(brew --repository)/Library/Taps/pixiv-cli-release/homebrew-staging"
brew tap-new "$staging_tap" --no-git
brew trust --tap "$staging_tap"
cp "staging-formula/$formula_name.rb" "$tap_dir/Formula/$formula_name.rb"
brew install --formula "$staging_tap/$formula_name"
pixiv version --json | python3 -c 'import json, sys; actual = json.load(sys.stdin)["version"]; expected = sys.argv[1]; assert actual == expected, f"version {actual!r} != {expected!r}"' "$RELEASE_TAG"`

	if err := requireRequiredJobExecution(job, "verify_homebrew_formula job"); err != nil {
		return err
	}
	if err := requireNoEnvironment(job, "verify_homebrew_formula job"); err != nil {
		return err
	}
	if err := requireOnlyMappingKeys(job, "name", "needs", "runs-on", "permissions", "strategy", "steps"); err != nil {
		return fmt.Errorf("verify_homebrew_formula job: %w", err)
	}
	if err := requireScalar(job, "needs", "render_homebrew_formula"); err != nil {
		return fmt.Errorf("verify_homebrew_formula job: %w", err)
	}
	if err := requireScalar(job, "runs-on", "${{ matrix.runner }}"); err != nil {
		return fmt.Errorf("verify_homebrew_formula job: %w", err)
	}
	if err := requireContentsPermission(job, "read"); err != nil {
		return fmt.Errorf("verify_homebrew_formula job: %w", err)
	}
	strategy, ok := mappingValue(job, "strategy")
	if !ok || requireOnlyMappingKeys(strategy, "fail-fast", "matrix") != nil || requireScalar(strategy, "fail-fast", "false") != nil {
		return errors.New("verify_homebrew_formula strategy must use fail-fast false and the exact four-target matrix")
	}
	matrix, _ := mappingValue(strategy, "matrix")
	if err := checkHomebrewMatrix(matrix); err != nil {
		return err
	}
	steps, err := jobSteps(job)
	if err != nil || len(steps) != 2 {
		return errors.New("verify_homebrew_formula job must contain only the formula download and native install gate")
	}
	if err := requireExactActionStep(steps[0], "staging formula download", downloadArtifactAction, map[string]string{
		"name": "staging-homebrew-formula",
		"path": "staging-formula",
	}); err != nil {
		return err
	}
	return requireCanonicalRunStep(steps[1], "Homebrew native install gate", verifyCommands)
}

func checkHomebrewMatrix(matrix *yaml.Node) error {
	if err := requireOnlyMappingKeys(matrix, "include"); err != nil {
		return errors.New("verify_homebrew_formula matrix must contain exactly the four native targets")
	}
	include, ok := mappingValue(matrix, "include")
	if !ok || include.Kind != yaml.SequenceNode || len(include.Content) != len(homebrewMatrixTargets) {
		return errors.New("verify_homebrew_formula matrix must contain exactly the four native targets")
	}
	seen := make(map[string]struct{}, len(include.Content))
	for _, entry := range include.Content {
		if err := requireOnlyMappingKeys(entry, "runner", "os", "arch"); err != nil {
			return errors.New("verify_homebrew_formula matrix must contain exactly the four native targets")
		}
		fields := make([]string, 0, 3)
		for _, key := range []string{"runner", "os", "arch"} {
			value, ok := mappingValue(entry, key)
			if !ok || value.Kind != yaml.ScalarNode {
				return errors.New("verify_homebrew_formula matrix must contain exactly the four native targets")
			}
			fields = append(fields, value.Value)
		}
		identity := strings.Join(fields, "|")
		if _, ok := homebrewMatrixTargets[identity]; !ok {
			return errors.New("verify_homebrew_formula matrix must contain exactly the four native targets")
		}
		if _, duplicate := seen[identity]; duplicate {
			return errors.New("verify_homebrew_formula matrix must contain exactly the four native targets")
		}
		seen[identity] = struct{}{}
	}
	return nil
}

func checkDeployHomebrewJob(job *yaml.Node) error {
	const prepareCommands = `
set -euo pipefail
formula_name=$(cat staging-formula/formula-name)
case "$formula_name" in
pixiv-cli|pixiv-cli-beta) ;;
*) exit 1 ;;
esac
test -f "staging-formula/$formula_name.rb"
test "$(find staging-formula -maxdepth 1 -type f -print | LC_ALL=C sort)" = "$(printf '%s\n%s\n' staging-formula/formula-name "staging-formula/$formula_name.rb" | LC_ALL=C sort)"
tap_dir="$RUNNER_TEMP/homebrew-tap"
git clone https://github.com/FlanChanXwO/homebrew-tap.git "$tap_dir"
git -C "$tap_dir" config user.name github-actions[bot]
git -C "$tap_dir" config user.email 41898282+github-actions[bot]@users.noreply.github.com
mkdir -p "$tap_dir/Formula"
cp "staging-formula/$formula_name.rb" "$tap_dir/Formula/$formula_name.rb"
git -C "$tap_dir" add -- "Formula/$formula_name.rb"
test "$(git -C "$tap_dir" diff --cached --name-only)" = "Formula/$formula_name.rb"
test -z "$(git -C "$tap_dir" status --porcelain | sed -n '\|^?? |p')"
git -C "$tap_dir" commit -m "${RELEASE_TAG}: update $formula_name formula"`
	const pushCommands = `
set -eu
umask 077
key_path="$RUNNER_TEMP/homebrew-tap-deploy-key"
trap 'rm -f "$key_path"' EXIT HUP INT TERM
test -n "$HOMEBREW_TAP_DEPLOY_KEY"
printf '%s\n' "$HOMEBREW_TAP_DEPLOY_KEY" > "$key_path"
chmod 600 "$key_path"
tap_dir="$RUNNER_TEMP/homebrew-tap"
git -C "$tap_dir" remote set-url origin git@github.com:FlanChanXwO/homebrew-tap.git
GIT_SSH_COMMAND="ssh -i $key_path -o IdentitiesOnly=yes -o StrictHostKeyChecking=yes -o UserKnownHostsFile=$GITHUB_WORKSPACE/templates/homebrew/github.com-known-hosts" \
git -C "$tap_dir" push origin HEAD:main`

	if err := requireRequiredJobExecution(job, "deploy_homebrew_tap job"); err != nil {
		return err
	}
	if err := requireOnlyMappingKeys(job, "name", "needs", "runs-on", "environment", "permissions", "steps"); err != nil {
		return fmt.Errorf("deploy_homebrew_tap job: %w", err)
	}
	if err := requireScalar(job, "needs", "verify_homebrew_formula"); err != nil {
		return fmt.Errorf("deploy_homebrew_tap job: %w", err)
	}
	if err := requireScalar(job, "runs-on", "ubuntu-24.04"); err != nil {
		return fmt.Errorf("deploy_homebrew_tap job: %w", err)
	}
	if err := requireScalar(job, "environment", "release"); err != nil {
		return errors.New("deploy_homebrew_tap environment must be release")
	}
	if err := requireContentsPermission(job, "read"); err != nil {
		return fmt.Errorf("deploy_homebrew_tap job: %w", err)
	}
	steps, err := jobSteps(job)
	if err != nil || len(steps) != 4 {
		return errors.New("deploy_homebrew_tap job must contain only the canonical checkout, formula download, commit, and final push steps")
	}
	if err := requireCanonicalCheckout(steps[0], "deploy_homebrew_tap job", checkoutWithRequirement{"persist-credentials", "false"}, checkoutWithRequirement{"ref", "${{ env.RELEASE_TAG }}"}); err != nil {
		return err
	}
	if err := requireExactActionStep(steps[1], "deploy formula download", downloadArtifactAction, map[string]string{
		"name": "staging-homebrew-formula",
		"path": "staging-formula",
	}); err != nil {
		return err
	}
	if err := requireCanonicalRunStep(steps[2], "tap one-formula commit step", prepareCommands); err != nil {
		return err
	}
	if err := requireCanonicalRunStepWithEnvironment(steps[3], "tap final protected push step", pushCommands); err != nil {
		return err
	}
	return nil
}

func requireCanonicalRunStepWithEnvironment(step *yaml.Node, context, canonical string) error {
	if err := requireOnlyMappingKeys(step, "name", "shell", "env", "run"); err != nil {
		return fmt.Errorf("%s must be the canonical direct bash step", context)
	}
	if err := requireScalar(step, "shell", "bash"); err != nil {
		return fmt.Errorf("%s must be the canonical direct bash step", context)
	}
	if !equalCommands(splitCommands(requireRunValue(step)), splitCommands(canonical)) {
		return fmt.Errorf("%s must use the required direct command sequence", context)
	}
	return nil
}

func requireUnconditionalAncestryGate(step *yaml.Node) error {
	for _, key := range []string{"continue-on-error", "if"} {
		if _, exists := mappingValue(step, key); exists {
			return errors.New("verify_release_source default-branch ancestry gate must not define if or continue-on-error")
		}
	}
	if err := requireOnlyMappingKeys(step, "name", "shell", "env", "run"); err != nil {
		return errors.New("verify_release_source default-branch ancestry gate must be a direct bash step")
	}
	if err := requireScalar(step, "shell", "bash"); err != nil {
		return errors.New("verify_release_source default-branch ancestry gate must use shell bash")
	}
	if commands := splitCommands(requireRunValue(step)); !equalCommands(commands, []string{
		"set -eu",
		"git show-ref --verify --quiet \"refs/remotes/origin/$DEFAULT_BRANCH\"",
		"git merge-base --is-ancestor HEAD \"origin/$DEFAULT_BRANCH\"",
	}) {
		return errors.New("verify_release_source default-branch ancestry gate must use the required direct command sequence")
	}
	return nil
}

func checkReleaseChannelBinding(releaseStep *yaml.Node) error {
	// 将受信 channel command substitution、case 分支、数组赋值和 gh release create 固定在同一
	// run 中。workflow 不引入可重写的 channel 变量，避免分类结果在分支前被其它 shell 命令覆盖。
	lines := splitCommands(requireRunValue(releaseStep))
	channelCase := "case \"$(go run ./scripts/releaseassets channel --version \"${RELEASE_TAG#v}\")\" in"
	if countCommand(lines, channelCase) != 1 {
		return errors.New("release publishing step must classify with the direct releaseassets case expression")
	}
	sequence := []string{
		"prerelease=()",
		channelCase,
		"stable)",
		";;",
		"prerelease)",
		"prerelease+=(--prerelease)",
		";;",
		"*)",
		"exit 1",
		";;",
		"esac",
		"gh release create \"$RELEASE_TAG\" \\",
	}
	position := -1
	for _, command := range sequence {
		position = commandIndexAfter(lines, command, position)
		if position < 0 {
			return errors.New("release publishing step must bind releaseassets channel to the prerelease flag")
		}
	}
	if commandIndexAfter(lines, "\"${prerelease[@]}\" \\", position) < 0 {
		return errors.New("release publishing step must pass the channel-derived prerelease flag to gh release create")
	}
	if err := requireApprovedChannelCaseCommands(lines, channelCase); err != nil {
		return err
	}
	if countCommand(lines, "prerelease=()") != 1 || countCommand(lines, "prerelease+=(--prerelease)") != 1 || countRunFragment(releaseStep, "--prerelease") != 1 {
		return errors.New("release publishing step must not hard-code or reassign the prerelease flag")
	}
	for _, command := range lines {
		if !strings.Contains(command, "prerelease") {
			continue
		}
		switch command {
		case "prerelease=()", "prerelease)", "prerelease+=(--prerelease)", "\"${prerelease[@]}\" \\":
			continue
		default:
			return errors.New("release publishing step must not hard-code or reassign the prerelease flag")
		}
	}
	return nil
}

func requireApprovedChannelCaseCommands(commands []string, channelCase string) error {
	releaseCreateCommand := `gh release create "$RELEASE_TAG" \`
	if len(commands) == 0 || commands[0] != "set -eu" {
		return errors.New("release publishing step must use only the approved channel case commands")
	}
	start := 0
	end := commandIndexAfter(commands, releaseCreateCommand, start)
	if start < 0 || end < start {
		return errors.New("release publishing step must use only the approved channel case commands")
	}
	for index := start; index <= end; index++ {
		switch commands[index] {
		case "set -eu", "prerelease=()", channelCase, "stable)", ";;", "prerelease)", "prerelease+=(--prerelease)", "*)", "printf '%s\\n' 'unexpected release classification' >&2", "exit 1", "esac", releaseCreateCommand:
			continue
		default:
			return errors.New("release publishing step must use only the approved channel case commands")
		}
	}
	return nil
}

func checkSigningSecretReachability(validate, build, productionBuild, verifyReleaseSource, publish *yaml.Node, publishSteps []*yaml.Node, signingIndex int) error {
	// release Environment 的 secret 只应在 verify_release_source 成功后才由 publish job 注入；
	// 非发布 job、publish 的 job 级字段和非签名 step 都不允许引用 secrets，防止同一 job
	// 启动即注入的 credential 在 shell gate 或其它步骤被提前访问。
	for _, job := range []*yaml.Node{validate, build, productionBuild, verifyReleaseSource} {
		if containsSigningSecretReference(job) {
			return errors.New("non-release job must not reference secrets")
		}
	}
	for index := 0; index+1 < len(publish.Content); index += 2 {
		if publish.Content[index].Value != "steps" && containsSigningSecretReference(publish.Content[index+1]) {
			return errors.New("publish job must not reference secrets outside its signing metadata step")
		}
	}
	for index, step := range publishSteps {
		if index != signingIndex && containsSigningSecretReference(step) {
			return errors.New("publish job must not reference secrets outside its signing metadata step")
		}
		if index == signingIndex && containsSigningSecretReferenceOutsideEnvironment(step) {
			return errors.New("signing metadata step must reference secrets only through its expected environment")
		}
	}
	return nil
}

func checkHomebrewSecretReachability(render, verify, deploy *yaml.Node) error {
	if containsSigningSecretReference(render) || containsSigningSecretReference(verify) {
		return errors.New("Homebrew render and install jobs must not reference secrets")
	}
	for index := 0; index+1 < len(deploy.Content); index += 2 {
		if deploy.Content[index].Value != "steps" && containsSigningSecretReference(deploy.Content[index+1]) {
			return errors.New("deploy_homebrew_tap job must not reference secrets outside its final push step")
		}
	}
	steps, err := jobSteps(deploy)
	if err != nil {
		return nil
	}
	for index, step := range steps {
		if index != len(steps)-1 && containsSigningSecretReference(step) {
			return errors.New("deploy_homebrew_tap job must not reference secrets outside its final push step")
		}
		if index == len(steps)-1 && containsSigningSecretReferenceOutsideEnvironment(step) {
			return errors.New("tap final push step must reference its secret only through the expected environment")
		}
	}
	if len(steps) == 0 {
		return nil
	}
	env, ok := mappingValue(steps[len(steps)-1], "env")
	if !ok || requireOnlyMappingKeys(env, "HOMEBREW_TAP_DEPLOY_KEY") != nil {
		return errors.New("tap final push step must declare only HOMEBREW_TAP_DEPLOY_KEY")
	}
	value, ok := mappingValue(env, "HOMEBREW_TAP_DEPLOY_KEY")
	if !ok || value.Kind != yaml.ScalarNode || !expectedSigningSecretExpression("HOMEBREW_TAP_DEPLOY_KEY").MatchString(value.Value) {
		return errors.New("tap final push step must use the protected HOMEBREW_TAP_DEPLOY_KEY secret")
	}
	return nil
}

func checkSigningStep(step *yaml.Node) error {
	env, ok := mappingValue(step, "env")
	if !ok || env.Kind != yaml.MappingNode {
		return errors.New("signing-secret step must declare its secret environment")
	}
	if err := requireOnlyMappingKeys(env, "RELEASE_SIGNING_PRIVATE_KEY", "RELEASE_SIGNING_KEY_ID"); err != nil {
		return errors.New("signing-secret step must declare only its expected signing secrets")
	}
	if err := requireExpectedSigningSecret(env, "RELEASE_SIGNING_PRIVATE_KEY"); err != nil {
		return errors.New("signing-secret step must use the protected private-key secret")
	}
	if err := requireExpectedSigningSecret(env, "RELEASE_SIGNING_KEY_ID"); err != nil {
		return errors.New("signing-secret step must use the protected key-ID secret")
	}
	for _, command := range []string{
		"set -eu",
		"umask 077",
		"test -n \"$RELEASE_SIGNING_PRIVATE_KEY\"",
		"test -n \"$RELEASE_SIGNING_KEY_ID\"",
		"printf '%s' \"$RELEASE_SIGNING_PRIVATE_KEY\" > \"$key_path\"",
	} {
		if !hasStepCommand(step, command) {
			return fmt.Errorf("signing-secret step must run %s", command)
		}
	}
	run, _ := mappingValue(step, "run")
	if err := requireRunFragments(step, "signing-secret step", "trap 'rm -f \"$key_path\"'", "go run ./scripts/releaseassets finalize", "--input-dir dist", "--output-dir release", "--private-key \"$key_path\""); err != nil {
		return err
	}
	if strings.Contains(run.Value, "set -x") || strings.Contains(run.Value, "echo $RELEASE_SIGNING") {
		return errors.New("signing-secret step must not print signing secret values")
	}
	return nil
}

func requireExpectedSigningSecret(env *yaml.Node, key string) error {
	value, ok := mappingValue(env, key)
	if !ok || value.Kind != yaml.ScalarNode || !expectedSigningSecretExpression(key).MatchString(value.Value) {
		return fmt.Errorf("%s must be its exact signing secret expression", key)
	}
	return nil
}

func expectedSigningSecretExpression(key string) *regexp.Regexp {
	quotedKey := regexp.QuoteMeta(key)
	return regexp.MustCompile(`^\$\{\{\s*secrets\s*(?:\.\s*` + quotedKey + `|\[\s*['"]` + quotedKey + `['"]\s*\])\s*\}\}$`)
}

func containsSigningSecretReference(node *yaml.Node) bool {
	if node == nil {
		return false
	}
	if node.Kind == yaml.ScalarNode && secretReferencePattern.MatchString(node.Value) {
		return true
	}
	for _, child := range node.Content {
		if containsSigningSecretReference(child) {
			return true
		}
	}
	return false
}

func containsSigningSecretReferenceOutsideEnvironment(step *yaml.Node) bool {
	if step == nil || step.Kind != yaml.MappingNode {
		return containsSigningSecretReference(step)
	}
	for index := 0; index+1 < len(step.Content); index += 2 {
		if step.Content[index].Value == "env" {
			continue
		}
		if containsSigningSecretReference(step.Content[index+1]) {
			return true
		}
	}
	return false
}

func requireCredentialFreeCheckout(job *yaml.Node, jobName string) error {
	steps, err := jobSteps(job)
	if err != nil {
		return fmt.Errorf("%s must have exactly one canonical checkout", jobName)
	}
	checkoutCount := 0
	for _, step := range steps {
		uses, hasUses := mappingValue(step, "uses")
		if !hasUses || uses.Kind != yaml.ScalarNode || !strings.HasPrefix(uses.Value, "actions/checkout@") {
			continue
		}
		checkoutCount++
		if err := requireCanonicalCheckout(step, jobName, checkoutWithRequirement{"persist-credentials", "false"}); err != nil {
			return err
		}
	}
	if checkoutCount != 1 {
		return fmt.Errorf("%s must have exactly one canonical checkout", jobName)
	}
	return nil
}

func signingStepIndex(steps []*yaml.Node) (int, *yaml.Node) {
	for index, step := range steps {
		env, ok := mappingValue(step, "env")
		if !ok {
			continue
		}
		if _, exists := mappingValue(env, "RELEASE_SIGNING_PRIVATE_KEY"); exists {
			return index, step
		}
	}
	return -1, nil
}

func mappingValue(mapping *yaml.Node, key string) (*yaml.Node, bool) {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return nil, false
	}
	for index := 0; index+1 < len(mapping.Content); index += 2 {
		if mapping.Content[index].Value == key {
			return mapping.Content[index+1], true
		}
	}
	return nil, false
}

func requireOnlyMappingKeys(mapping *yaml.Node, keys ...string) error {
	if mapping == nil || mapping.Kind != yaml.MappingNode || len(mapping.Content) != len(keys)*2 {
		return errors.New("must contain exactly the required keys")
	}
	allowed := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		allowed[key] = struct{}{}
	}
	for index := 0; index+1 < len(mapping.Content); index += 2 {
		if _, ok := allowed[mapping.Content[index].Value]; !ok {
			return errors.New("must contain exactly the required keys")
		}
	}
	return nil
}

func requireScalar(mapping *yaml.Node, key, want string) error {
	value, ok := mappingValue(mapping, key)
	if !ok || value.Kind != yaml.ScalarNode || value.Value != want {
		return fmt.Errorf("%s must equal %q", key, want)
	}
	return nil
}

func requireContentsPermission(job *yaml.Node, level string) error {
	permissions, ok := mappingValue(job, "permissions")
	if !ok || permissions.Kind != yaml.MappingNode || len(permissions.Content) != 2 {
		return fmt.Errorf("permissions must contain only contents: %s", level)
	}
	if err := requireScalar(permissions, "contents", level); err != nil {
		return fmt.Errorf("permissions must contain only contents: %s", level)
	}
	return nil
}

func jobSteps(job *yaml.Node) ([]*yaml.Node, error) {
	steps, ok := mappingValue(job, "steps")
	if !ok || steps.Kind != yaml.SequenceNode || len(steps.Content) == 0 {
		return nil, errors.New("must contain a non-empty steps sequence")
	}
	return steps.Content, nil
}

func hasCommandInWorkingDirectory(job *yaml.Node, directory, command string) bool {
	steps, err := jobSteps(job)
	if err != nil {
		return false
	}
	for _, step := range steps {
		workingDirectory, hasWorkingDirectory := mappingValue(step, "working-directory")
		run, hasRun := mappingValue(step, "run")
		if !hasWorkingDirectory || !hasRun || workingDirectory.Value != directory {
			continue
		}
		for _, line := range splitCommands(run.Value) {
			if line == command {
				return true
			}
		}
	}
	return false
}

func hasCommand(job *yaml.Node, directory, command string) bool {
	if directory != "" {
		return hasCommandInWorkingDirectory(job, directory, command)
	}
	steps, err := jobSteps(job)
	if err != nil {
		return false
	}
	for _, step := range steps {
		workingDirectory, hasWorkingDirectory := mappingValue(step, "working-directory")
		if hasWorkingDirectory && (workingDirectory.Kind != yaml.ScalarNode || workingDirectory.Value != ".") {
			continue
		}
		if hasStepCommand(step, command) {
			return true
		}
	}
	return false
}

func rootStepWithRunFragment(job *yaml.Node, fragment string) (*yaml.Node, bool) {
	steps, err := jobSteps(job)
	if err != nil {
		return nil, false
	}
	for _, step := range steps {
		workingDirectory, hasWorkingDirectory := mappingValue(step, "working-directory")
		if hasWorkingDirectory && (workingDirectory.Kind != yaml.ScalarNode || workingDirectory.Value != ".") {
			continue
		}
		run, hasRun := mappingValue(step, "run")
		if hasRun && run.Kind == yaml.ScalarNode && strings.Contains(run.Value, fragment) {
			return step, true
		}
	}
	return nil, false
}

func requireRunFragments(step *yaml.Node, context string, fragments ...string) error {
	run := requireRunValue(step)
	if run == "" {
		return fmt.Errorf("%s must have a run command", context)
	}
	for _, fragment := range fragments {
		if !strings.Contains(run, fragment) {
			return fmt.Errorf("%s must contain %s", context, fragment)
		}
	}
	return nil
}

func requireRunValue(step *yaml.Node) string {
	run, hasRun := mappingValue(step, "run")
	if !hasRun || run.Kind != yaml.ScalarNode {
		return ""
	}
	return run.Value
}

func commandIndexAfter(commands []string, want string, after int) int {
	for index := after + 1; index < len(commands); index++ {
		if commands[index] == want {
			return index
		}
	}
	return -1
}

func countCommand(commands []string, want string) int {
	count := 0
	for _, command := range commands {
		if command == want {
			count++
		}
	}
	return count
}

func countRunFragment(step *yaml.Node, fragment string) int {
	return strings.Count(requireRunValue(step), fragment)
}

func hasShellArgument(step *yaml.Node, argument string) bool {
	run, hasRun := mappingValue(step, "run")
	if !hasRun || run.Kind != yaml.ScalarNode {
		return false
	}
	for _, line := range strings.Split(run.Value, "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimSpace(strings.TrimSuffix(line, "\\"))
		if line == argument {
			return true
		}
	}
	return false
}

func hasStepCommand(step *yaml.Node, command string) bool {
	run, hasRun := mappingValue(step, "run")
	if !hasRun || run.Kind != yaml.ScalarNode {
		return false
	}
	for _, line := range splitCommands(run.Value) {
		if line == command {
			return true
		}
	}
	return false
}

func splitCommands(run string) []string {
	var commands []string
	for _, line := range strings.Split(run, "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			commands = append(commands, line)
		}
	}
	return commands
}

func equalCommands(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for index := range want {
		if got[index] != want[index] {
			return false
		}
	}
	return true
}
