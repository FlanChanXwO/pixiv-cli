package main

import (
	"errors"

	"github.com/FlanChanXwO/pixiv-cli/scripts/internal/workflowpolicy"
	"gopkg.in/yaml.v3"
)

func checkTagTrigger(root *yaml.Node) error {
	on, ok := workflowpolicy.MappingValue(root, "on")
	if !ok || on.Kind != yaml.MappingNode {
		return errors.New("workflow must have an on mapping")
	}
	if err := requireOnlyMappingKeys(on, "push", "workflow_dispatch"); err != nil {
		return errors.New("on must contain only push and workflow_dispatch triggers")
	}
	push, ok := workflowpolicy.MappingValue(on, "push")
	if !ok || push.Kind != yaml.MappingNode {
		return errors.New("on.push must be a mapping")
	}
	if err := requireOnlyMappingKeys(push, "tags"); err != nil {
		return errors.New("on.push must contain only tags")
	}
	tags, ok := workflowpolicy.MappingValue(push, "tags")
	if !ok || tags.Kind != yaml.SequenceNode || len(tags.Content) != 1 || tags.Content[0].Value != "v[0-9]*" {
		return errors.New("on.push.tags must equal [v[0-9]*]")
	}
	dispatch, ok := workflowpolicy.MappingValue(on, "workflow_dispatch")
	if !ok || requireOnlyMappingKeys(dispatch, "inputs") != nil {
		return errors.New("workflow_dispatch must contain only the exact release_tag input")
	}
	inputs, ok := workflowpolicy.MappingValue(dispatch, "inputs")
	if !ok || requireOnlyMappingKeys(inputs, "release_tag") != nil {
		return errors.New("workflow_dispatch must contain only the exact release_tag input")
	}
	releaseTag, ok := workflowpolicy.MappingValue(inputs, "release_tag")
	if !ok || requireOnlyMappingKeys(releaseTag, "description", "required", "type") != nil || workflowpolicy.RequireScalar(releaseTag, "required", "true") != nil || workflowpolicy.RequireScalar(releaseTag, "type", "string") != nil {
		return errors.New("workflow_dispatch release_tag must be a required string")
	}
	return nil
}

func checkRecoveryPolicy(root *yaml.Node) error {
	if err := checkTagTrigger(root); err != nil {
		return err
	}
	env, ok := workflowpolicy.MappingValue(root, "env")
	if !ok || requireOnlyMappingKeys(env, "RELEASE_TAG") != nil || workflowpolicy.RequireScalar(env, "RELEASE_TAG", "${{ github.event_name == 'workflow_dispatch' && inputs.release_tag || github.ref_name }}") != nil {
		return errors.New("workflow must bind RELEASE_TAG only to the push tag or required dispatch input")
	}
	jobs, ok := workflowpolicy.MappingValue(root, "jobs")
	if !ok {
		return errors.New("workflow must have jobs for recovery policy")
	}
	build, ok := workflowpolicy.MappingValue(jobs, "build")
	if !ok {
		return errors.New("workflow must have a build job for recovery policy")
	}
	buildEnv, ok := workflowpolicy.MappingValue(build, "env")
	if !ok || requireOnlyMappingKeys(buildEnv, "CC", "RUSTUP_TOOLCHAIN", "GIT_CONFIG_COUNT", "GIT_CONFIG_KEY_0", "GIT_CONFIG_VALUE_0") != nil || workflowpolicy.RequireScalar(buildEnv, "CC", "${{ matrix.cc }}") != nil || workflowpolicy.RequireScalar(buildEnv, "RUSTUP_TOOLCHAIN", "${{ matrix.rust_toolchain }}") != nil || workflowpolicy.RequireScalar(buildEnv, "GIT_CONFIG_COUNT", "1") != nil || workflowpolicy.RequireScalar(buildEnv, "GIT_CONFIG_KEY_0", "core.autocrlf") != nil || workflowpolicy.RequireScalar(buildEnv, "GIT_CONFIG_VALUE_0", "false") != nil {
		return errors.New("build job must bind the audited compiler, Rust toolchain, and immutable source byte checkout")
	}
	matrix := mustMappingPath(build, "strategy", "matrix")
	if matrix == nil {
		return errors.New("build matrix must contain exactly the six release targets")
	}
	if err := checkReleaseMatrix(matrix); err != nil {
		return err
	}
	productionBuild, ok := workflowpolicy.MappingValue(jobs, "build_production")
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
	return nil
}

func requireCanonicalBuildSteps(steps []*yaml.Node) error {
	if len(steps) != 15 {
		return errors.New("build job must contain exactly the 15 canonical test-only steps")
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
		{name: "Install the pinned native Rust toolchain", command: testRustInstallCommand},
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
	if err := workflowpolicy.RequireScalar(steps[7], "name", "Apply the audited test-only recovery overlay"); err != nil {
		return errors.New("recovery overlay must keep its canonical name")
	}
	if err := requireOverlayQualitySequence(steps, 7, 15); err != nil {
		return err
	}
	return nil
}

func requireRecoveryOverlayStep(step *yaml.Node) error {
	const commands = `
set -euo pipefail
test -z "$(git status --porcelain=v1 --untracked-files=all)"
test -z "$(git diff --cached --name-only)"
git archive --format=tar "$GITHUB_SHA" -- \
  .github/workflows/release.yml \
  scripts/installers/installers_test.go \
  scripts/internal/workflowpolicy/policy.go \
  scripts/releaseworkflow/build_policy.go \
  scripts/releaseworkflow/build_recovery_test.go \
  scripts/releaseworkflow/e2e_policy.go \
  scripts/releaseworkflow/e2e_policy_test.go \
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
  scripts/installers/installers_test.go \
  scripts/internal/workflowpolicy/policy.go \
  scripts/releaseworkflow/build_policy.go \
  scripts/releaseworkflow/build_recovery_test.go \
  scripts/releaseworkflow/e2e_policy.go \
  scripts/releaseworkflow/e2e_policy_test.go \
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
	if err := requireCanonicalConditionalRunStep(step, "recovery overlay", "github.event_name == 'workflow_dispatch'", commands); err != nil {
		return errors.New("recovery overlay must use only the exact audited Windows-compatible test paths and verifier")
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
		if err := requireOverlayQualityGate(step, gate.name, gate.command); err != nil {
			return errors.New("overlay quality sequence must preserve exact canonical gate commands and order")
		}
		if err := workflowpolicy.RequireScalar(step, "name", gate.name); err != nil {
			return errors.New("overlay quality sequence must preserve exact canonical gate names and order")
		}
	}
	return nil
}

// requireOverlayQualityGate 保留 race gate 的唯一客观例外：Go 1.26.3 不支持
// windows/arm64 race detector。其余五个 release 目标必须运行该 gate，条件本身也受策略固定。
func requireOverlayQualityGate(step *yaml.Node, name, command string) error {
	if command == "go test -race ./..." {
		return requireCanonicalConditionalRunStep(step, "overlay quality gate "+name, "matrix.goos != 'windows' || matrix.goarch != 'arm64'", command)
	}
	return requireCanonicalRunStep(step, "overlay quality gate "+name, command)
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
-o "$output" ./cmd/pixiv
if [ '${{ matrix.goos }}' = linux ]; then
go run ./scripts/linuxabi --binary "$output"
fi`
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
