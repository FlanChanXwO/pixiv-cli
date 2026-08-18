package releaseworkflow

import (
	"errors"
	"fmt"
	"strings"

	releasecontract "github.com/FlanChanXwO/pixiv-cli/scripts/internal/releasecontract"
	workflowyaml "github.com/FlanChanXwO/pixiv-cli/scripts/internal/workflow/yaml"
	"gopkg.in/yaml.v3"
)

// releaseMatrixTargets 将 runner、Go 平台、Rust target 与 release asset 名称绑为同一集合；
// Rust toolchain 另由 releasecontract 的共享 provenance 映射校验，避免 release 与 evidence 漂移。
var releaseMatrixTargets = map[string]struct{}{
	"macos-15-intel|darwin|amd64|x86_64-apple-darwin|darwin-amd64|clang":                    {},
	"macos-15|darwin|arm64|aarch64-apple-darwin|darwin-arm64|clang":                         {},
	"ubuntu-22.04|linux|amd64|x86_64-unknown-linux-gnu|linux-amd64|gcc":                     {},
	"ubuntu-22.04-arm|linux|arm64|aarch64-unknown-linux-gnu|linux-arm64|gcc":                {},
	"windows-2025|windows|amd64|x86_64-pc-windows-msvc|windows-amd64|clang -fuse-ld=lld":    {},
	"windows-11-arm|windows|arm64|aarch64-pc-windows-msvc|windows-arm64|clang -fuse-ld=lld": {},
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
	if err := workflowyaml.RequireScalar(job, "runs-on", "ubuntu-24.04"); err != nil {
		return fmt.Errorf("validate job: %w", err)
	}
	if _, exists := workflowyaml.MappingValue(job, "needs"); exists {
		return errors.New("validate job must not depend on another job")
	}
	if err := requireContentsPermission(job, "read"); err != nil {
		return fmt.Errorf("validate job: %w", err)
	}
	steps, err := jobSteps(job)
	if err != nil || len(steps) < 4 {
		return errors.New("validate job must contain the audited workflow checkout and release tag gates")
	}
	if err := requireCanonicalCheckout(steps[0], "validate job", checkoutWithRequirement{"fetch-depth", "0"}, checkoutWithRequirement{"persist-credentials", "false"}, checkoutWithRequirement{"ref", "${{ github.sha }}"}); err != nil {
		return err
	}
	if err := requireCanonicalNamedRunStep(steps[2], "Validate release SemVer", `go run ./scripts/cmd/releaseassets validate --version "${RELEASE_TAG#v}"`); err != nil {
		return err
	}
	validateStep := steps[3]
	if err := requireRunFragments(validateStep, "validate release tag step", "git show-ref --verify --quiet \"refs/tags/$RELEASE_TAG\"", "tag_skill=\"$RUNNER_TEMP/pixiv-cli-SKILL.md\"", "git show \"$RELEASE_TAG:skills/pixiv-cli/SKILL.md\" > \"$tag_skill\"", "go run ./scripts/cmd/releaseassets validate-source --version \"${RELEASE_TAG#v}\" --product-skill \"$tag_skill\"", "git merge-base --is-ancestor \"$tag_commit\" \"origin/$DEFAULT_BRANCH\"", "gh api --include \"repos/$GITHUB_REPOSITORY/releases/tags/$RELEASE_TAG\"", "HTTP/[0-9.]+ 404"); err != nil {
		return err
	}
	if !hasCommand(job, "", "go run ./scripts/cmd/releaseworkflow --workflow .github/workflows/release.yml") {
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
	if err := checkBuildEnvironment(job); err != nil {
		return err
	}
	if err := requireExactStringSequence(job, "needs", "validate", "e2e"); err != nil {
		return fmt.Errorf("build job: %w", err)
	}
	if err := workflowyaml.RequireScalar(job, "runs-on", "${{ matrix.runner }}"); err != nil {
		return fmt.Errorf("build job: %w", err)
	}
	if err := requireContentsPermission(job, "read"); err != nil {
		return fmt.Errorf("build job: %w", err)
	}
	steps, err := jobSteps(job)
	if err != nil || len(steps) == 0 {
		return errors.New("build job must contain steps")
	}
	if err := requireBuildCommandsPresent(job); err != nil {
		return err
	}
	if err := requireCanonicalCheckout(steps[0], "build job", checkoutWithRequirement{"fetch-depth", "0"}, checkoutWithRequirement{"persist-credentials", "false"}, checkoutWithRequirement{"ref", "${{ env.RELEASE_TAG }}"}); err != nil {
		return err
	}
	strategy, ok := workflowyaml.MappingValue(job, "strategy")
	if !ok || strategy.Kind != yaml.MappingNode {
		return errors.New("build job must have a matrix strategy")
	}
	if err := workflowyaml.RequireScalar(strategy, "fail-fast", "false"); err != nil {
		return fmt.Errorf("build strategy: %w", err)
	}
	matrix, ok := workflowyaml.MappingValue(strategy, "matrix")
	if !ok || matrix.Kind != yaml.MappingNode {
		return errors.New("build job must have a matrix")
	}
	if err := checkReleaseMatrix(matrix); err != nil {
		return err
	}

	if err := requireCanonicalBuildSteps(steps); err != nil {
		return err
	}
	return nil
}

func requireCanonicalBuildSteps(steps []*yaml.Node) error {
	if len(steps) != 14 {
		return errors.New("build job must contain exactly the 14 canonical quality steps")
	}
	if err := requireCanonicalCheckout(steps[0], "build checkout", checkoutWithRequirement{"fetch-depth", "0"}, checkoutWithRequirement{"persist-credentials", "false"}, checkoutWithRequirement{"ref", "${{ env.RELEASE_TAG }}"}); err != nil {
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
		{name: "Validate the exact immutable release source", command: `go run ./scripts/cmd/releaseassets validate-source --version "${RELEASE_TAG#v}" --product-skill skills/pixiv-cli/SKILL.md`},
		{name: "Install the pinned native Rust toolchain", command: testRustInstallCommand},
		{name: "Check vendored Rust sources", command: "sh scripts/test-rust-vendor.sh"},
		{name: "Check Rust formatting from vendored sources", command: "cargo fmt --check", directory: "internal/media/ugoira/rust"},
		{name: "Lint vendored Rust sources", command: "cargo clippy --locked --offline --all-targets -- -D warnings", directory: "internal/media/ugoira/rust"},
	} {
		step := steps[index+2]
		if hasExecutionOverride(step) {
			return fmt.Errorf("build quality gate %s must not define continue-on-error or if", gate.command)
		}
		if gate.directory == "" {
			if err := requireCanonicalNamedRunStep(step, gate.name, gate.command); err != nil {
				return fmt.Errorf("%s: %w", gate.command, err)
			}
			continue
		}
		if err := requireCanonicalNamedRunStepInDirectory(step, gate.name, gate.directory, gate.command); err != nil {
			return fmt.Errorf("%s: %w", gate.command, err)
		}
	}
	for index, gate := range []struct {
		name    string
		command string
	}{
		{name: "Test Go sources", command: "go test ./..."},
		{name: "Test Go sources with the race detector", command: "go test -race ./..."},
		{name: "Vet Go sources", command: "go vet ./..."},
		{name: "Audit bundled licenses", command: "go run ./scripts/cmd/licensebundle --check"},
		{name: "Test release packages", command: "sh scripts/test-package-release.sh"},
		{name: "Install the pinned pre-commit version", command: "python -m pip install --disable-pip-version-check pre-commit==4.6.0"},
		{name: "Run pre-commit checks", command: "python -m pre_commit run --all-files"},
	} {
		step := steps[index+7]
		if gate.command != "go test -race ./..." && hasExecutionOverride(step) {
			return fmt.Errorf("build quality gate %s must not define continue-on-error or if", gate.command)
		}
		if gate.command == "go test -race ./..." {
			if err := requireCanonicalConditionalRunStep(step, "build quality gate "+gate.name, "matrix.goos != 'windows' || matrix.goarch != 'arm64'", gate.command); err != nil {
				return fmt.Errorf("%s: %w", gate.command, err)
			}
			continue
		}
		if err := requireCanonicalNamedRunStep(step, gate.name, gate.command); err != nil {
			return fmt.Errorf("%s: %w", gate.command, err)
		}
	}
	return nil
}

func requireBuildCommandsPresent(job *yaml.Node) error {
	for _, gate := range []struct {
		name      string
		directory string
		command   string
	}{
		{name: "Validate the exact immutable release source", command: `go run ./scripts/cmd/releaseassets validate-source --version "${RELEASE_TAG#v}" --product-skill skills/pixiv-cli/SKILL.md`},
		{name: "Install the pinned native Rust toolchain", command: testRustInstallCommand},
		{name: "Check vendored Rust sources", command: "sh scripts/test-rust-vendor.sh"},
		{name: "Check Rust formatting from vendored sources", directory: "internal/media/ugoira/rust", command: "cargo fmt --check"},
		{name: "Lint vendored Rust sources", directory: "internal/media/ugoira/rust", command: "cargo clippy --locked --offline --all-targets -- -D warnings"},
		{name: "Test Go sources", command: "go test ./..."},
		{name: "Test Go sources with the race detector", command: "go test -race ./..."},
		{name: "Vet Go sources", command: "go vet ./..."},
		{name: "Audit bundled licenses", command: "go run ./scripts/cmd/licensebundle --check"},
		{name: "Test release packages", command: "sh scripts/test-package-release.sh"},
		{name: "Install the pinned pre-commit version", command: "python -m pip install --disable-pip-version-check pre-commit==4.6.0"},
		{name: "Run pre-commit checks", command: "python -m pre_commit run --all-files"},
	} {
		if !hasCommand(job, gate.directory, gate.command) {
			return fmt.Errorf("%s (%s) must be present", gate.name, gate.command)
		}
	}
	return nil
}

func hasExecutionOverride(step *yaml.Node) bool {
	for _, key := range []string{"continue-on-error", "if"} {
		if _, exists := workflowyaml.MappingValue(step, key); exists {
			return true
		}
	}
	return false
}

func checkBuildEnvironment(job *yaml.Node) error {
	env, ok := workflowyaml.MappingValue(job, "env")
	if !ok || requireOnlyMappingKeys(env, "CC", "RUSTUP_TOOLCHAIN", "GIT_CONFIG_COUNT", "GIT_CONFIG_KEY_0", "GIT_CONFIG_VALUE_0") != nil || workflowyaml.RequireScalar(env, "CC", "${{ matrix.cc }}") != nil || workflowyaml.RequireScalar(env, "RUSTUP_TOOLCHAIN", "${{ matrix.rust_toolchain }}") != nil || workflowyaml.RequireScalar(env, "GIT_CONFIG_COUNT", "1") != nil || workflowyaml.RequireScalar(env, "GIT_CONFIG_KEY_0", "core.autocrlf") != nil || workflowyaml.RequireScalar(env, "GIT_CONFIG_VALUE_0", "false") != nil {
		return errors.New("build job must bind the audited compiler, Rust toolchain, and immutable source byte checkout")
	}
	return nil
}

// checkProductionBuildJob 将最终资产放入独立 runner，并只读取 immutable tag。
func checkProductionBuildJob(job *yaml.Node) error {
	if err := requireRequiredJobExecution(job, "build_production job"); err != nil {
		return err
	}
	if err := requireNoEnvironment(job, "build_production job"); err != nil {
		return err
	}
	if containsScalarFragment(job, "GITHUB_SHA") {
		return errors.New("build_production job must not read the workflow GITHUB_SHA")
	}
	if err := requireOnlyMappingKeys(job, "name", "needs", "runs-on", "permissions", "env", "strategy", "steps"); err != nil {
		return fmt.Errorf("build_production job: %w", err)
	}
	if err := workflowyaml.RequireScalar(job, "name", "Build production ${{ matrix.goos }}/${{ matrix.goarch }}"); err != nil {
		return fmt.Errorf("build_production job: %w", err)
	}
	if err := workflowyaml.RequireScalar(job, "needs", "build"); err != nil {
		return fmt.Errorf("build_production job: %w", err)
	}
	if err := workflowyaml.RequireScalar(job, "runs-on", "${{ matrix.runner }}"); err != nil {
		return fmt.Errorf("build_production job: %w", err)
	}
	if err := requireContentsPermission(job, "read"); err != nil {
		return fmt.Errorf("build_production job: %w", err)
	}
	env, ok := workflowyaml.MappingValue(job, "env")
	if !ok || requireOnlyMappingKeys(env, "CC", "RUSTUP_TOOLCHAIN") != nil || workflowyaml.RequireScalar(env, "CC", "${{ matrix.cc }}") != nil || workflowyaml.RequireScalar(env, "RUSTUP_TOOLCHAIN", "${{ matrix.rust_toolchain }}") != nil {
		return errors.New("build_production job must bind CC and the Rust toolchain from the audited release policy")
	}
	strategy, ok := workflowyaml.MappingValue(job, "strategy")
	if !ok || requireOnlyMappingKeys(strategy, "fail-fast", "matrix") != nil || workflowyaml.RequireScalar(strategy, "fail-fast", "false") != nil {
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
	if err := requireCanonicalNamedRunStep(steps[2], "Validate the exact immutable production source", `go run ./scripts/cmd/releaseassets validate-source --version "${RELEASE_TAG#v}" --product-skill skills/pixiv-cli/SKILL.md`); err != nil {
		return err
	}
	if err := requireCanonicalNamedRunStep(steps[3], "Install the pinned native Rust toolchain", prodRustInstallCommand); err != nil {
		return err
	}
	if err := requireProductionRebuildStep(steps[4]); err != nil || workflowyaml.RequireScalar(steps[4], "name", "Rebuild the selected static library from the immutable tag") != nil {
		return errors.New("build_production job must rebuild the selected static library from the immutable tag")
	}
	if err := requireCanonicalNamedRunStep(steps[5], "Check the generated diff", "git diff --check"); err != nil {
		return err
	}
	if err := requireProductionBuildStep(steps[6]); err != nil || workflowyaml.RequireScalar(steps[6], "name", "Build the versioned native executable") != nil {
		return errors.New("build_production job must build the tag-bound executable")
	}
	if err := requireProductionPackageStep(steps[7]); err != nil || workflowyaml.RequireScalar(steps[7], "name", "Package the fixed-name platform asset") != nil {
		return errors.New("build_production job must package the tag-bound executable")
	}
	return requireExactActionStep(steps[8], "verified release build artifact upload", uploadArtifactAction, map[string]string{
		"name":              "verified-release-${{ matrix.artifact }}",
		"path":              "dist/pixiv-cli_*",
		"if-no-files-found": "error",
		"retention-days":    "1",
	})
}

func requireProductionRebuildStep(step *yaml.Node) error {
	const commands = `
set -euo pipefail
test "$(git rev-parse HEAD)" = "$(git rev-parse "$RELEASE_TAG^{commit}")"
test -z "$(git status --porcelain --untracked-files=all)"
test -z "$(git clean -ndx)"
bash scripts/build-staticlibs.sh --target '${{ matrix.rust_target }}'
git restore --source="$RELEASE_TAG^{commit}" -- internal/media/ugoira/rust/staticlib/manifest.json
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
-ldflags "-X github.com/FlanChanXwO/pixiv-cli/internal/shared/buildinfo.Version=${RELEASE_TAG}" \
-o "$output" ./cmd/pixiv
if [ '${{ matrix.goos }}' = linux ]; then
go run ./scripts/cmd/linuxabi --binary "$output"
fi`
	if err := requireCanonicalRunStep(step, "production versioned binary build", commands); err != nil {
		return errors.New("production build must use the exact tag-bound version and output command sequence")
	}
	return nil
}

func requireProductionPackageStep(step *yaml.Node) error {
	const commands = `
go run ./scripts/cmd/releaseassets package \
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

func checkReleaseMatrix(matrix *yaml.Node) error {
	if err := requireOnlyMappingKeys(matrix, "include"); err != nil {
		return errors.New("build matrix must contain only the six release targets")
	}
	include, ok := workflowyaml.MappingValue(matrix, "include")
	if !ok || include.Kind != yaml.SequenceNode || len(include.Content) != len(releaseMatrixTargets) {
		return errors.New("build matrix must contain exactly the six release targets")
	}
	seen := make(map[string]struct{}, len(include.Content))
	for _, entry := range include.Content {
		if entry.Kind != yaml.MappingNode {
			return errors.New("build matrix must contain exactly the six release targets")
		}
		if err := requireOnlyMappingKeys(entry, "runner", "goos", "goarch", "rust_target", "rust_toolchain", "artifact", "cc"); err != nil {
			return errors.New("build matrix entries must contain only the canonical release target fields")
		}
		target, _ := workflowyaml.MappingValue(entry, "rust_target")
		toolchain, _ := workflowyaml.MappingValue(entry, "rust_toolchain")
		wantToolchain, supported := releasecontract.PinnedRustToolchain(target.Value)
		if !supported || toolchain.Value != wantToolchain {
			return errors.New("build matrix must use the release-pinned Rust toolchain for every target")
		}
		fields := make([]string, 0, 6)
		for _, key := range []string{"runner", "goos", "goarch", "rust_target", "artifact", "cc"} {
			value, ok := workflowyaml.MappingValue(entry, key)
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
