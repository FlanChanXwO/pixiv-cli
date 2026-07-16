package main

import (
	"errors"
	"fmt"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/scripts/internal/workflowpolicy"
	"gopkg.in/yaml.v3"
)

// releaseMatrixTargets 将 runner、Go 平台、Rust target、生成已提交 staticlib 的 Rust toolchain
// 和 release asset 名称绑为同一集合，防止任一字段的局部改动让六平台发布遗漏或错配。
var releaseMatrixTargets = map[string]struct{}{
	"macos-15-intel|darwin|amd64|x86_64-apple-darwin|1.96.0|darwin-amd64|clang":                    {},
	"macos-15|darwin|arm64|aarch64-apple-darwin|1.96.1|darwin-arm64|clang":                         {},
	"ubuntu-24.04|linux|amd64|x86_64-unknown-linux-gnu|1.96.1|linux-amd64|gcc":                     {},
	"ubuntu-24.04-arm|linux|arm64|aarch64-unknown-linux-gnu|1.96.1|linux-arm64|gcc":                {},
	"windows-2025|windows|amd64|x86_64-pc-windows-msvc|1.96.0|windows-amd64|clang -fuse-ld=lld":    {},
	"windows-11-arm|windows|arm64|aarch64-pc-windows-msvc|1.96.1|windows-arm64|clang -fuse-ld=lld": {},
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
	if err := workflowpolicy.RequireScalar(job, "runs-on", "ubuntu-24.04"); err != nil {
		return fmt.Errorf("validate job: %w", err)
	}
	if _, exists := workflowpolicy.MappingValue(job, "needs"); exists {
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
	if err := requireCanonicalNamedRunStep(steps[2], "Validate release SemVer", `go run ./scripts/releaseassets validate --version "${RELEASE_TAG#v}"`); err != nil {
		return err
	}
	validateStep := steps[3]
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
	if err := workflowpolicy.RequireScalar(job, "needs", "validate"); err != nil {
		return fmt.Errorf("build job: %w", err)
	}
	if err := workflowpolicy.RequireScalar(job, "runs-on", "${{ matrix.runner }}"); err != nil {
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
	strategy, ok := workflowpolicy.MappingValue(job, "strategy")
	if !ok || strategy.Kind != yaml.MappingNode {
		return errors.New("build job must have a matrix strategy")
	}
	if err := workflowpolicy.RequireScalar(strategy, "fail-fast", "false"); err != nil {
		return fmt.Errorf("build strategy: %w", err)
	}
	matrix, ok := workflowpolicy.MappingValue(strategy, "matrix")
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
		{command: "go vet ./..."},
		{command: "go run ./scripts/licensebundle --check"},
		{command: "sh scripts/test-package-release.sh"},
		{command: "python -m pip install --disable-pip-version-check pre-commit==4.6.0"},
		{command: "python -m pre_commit run --all-files"},
	} {
		if err := requireIndependentQualityGate(job, gate.workingDirectory, gate.command); err != nil {
			return err
		}
	}
	if err := requireConditionalRaceQualityGate(job); err != nil {
		return err
	}
	return nil
}

func requireConditionalRaceQualityGate(job *yaml.Node) error {
	steps, err := jobSteps(job)
	if err != nil {
		return errors.New("build must run go test -race ./...")
	}
	var matches []*yaml.Node
	for _, step := range steps {
		if equalCommands(splitCommands(requireRunValue(step)), splitCommands("go test -race ./...")) {
			matches = append(matches, step)
		}
	}
	if len(matches) != 1 {
		return errors.New("build must run go test -race ./... in exactly one quality gate step")
	}
	return requireCanonicalConditionalRunStep(matches[0], "build quality gate go test -race ./...", "matrix.goos != 'windows' || matrix.goarch != 'arm64'", "go test -race ./...")
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
	if err := workflowpolicy.RequireScalar(job, "name", "Build production ${{ matrix.goos }}/${{ matrix.goarch }}"); err != nil {
		return fmt.Errorf("build_production job: %w", err)
	}
	if err := workflowpolicy.RequireScalar(job, "needs", "build"); err != nil {
		return fmt.Errorf("build_production job: %w", err)
	}
	if err := workflowpolicy.RequireScalar(job, "runs-on", "${{ matrix.runner }}"); err != nil {
		return fmt.Errorf("build_production job: %w", err)
	}
	if err := requireContentsPermission(job, "read"); err != nil {
		return fmt.Errorf("build_production job: %w", err)
	}
	env, ok := workflowpolicy.MappingValue(job, "env")
	if !ok || requireOnlyMappingKeys(env, "CC", "RUSTUP_TOOLCHAIN") != nil || workflowpolicy.RequireScalar(env, "CC", "${{ matrix.cc }}") != nil || workflowpolicy.RequireScalar(env, "RUSTUP_TOOLCHAIN", "${{ matrix.rust_toolchain }}") != nil {
		return errors.New("build_production job must bind CC and the Rust toolchain from the audited release policy")
	}
	strategy, ok := workflowpolicy.MappingValue(job, "strategy")
	if !ok || requireOnlyMappingKeys(strategy, "fail-fast", "matrix") != nil || workflowpolicy.RequireScalar(strategy, "fail-fast", "false") != nil {
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
	if err := requireCanonicalNamedRunStep(steps[3], "Install the pinned native Rust toolchain", prodRustInstallCommand); err != nil {
		return err
	}
	if err := requireProductionRebuildStep(steps[4]); err != nil || workflowpolicy.RequireScalar(steps[4], "name", "Rebuild the selected static library from the immutable tag") != nil {
		return errors.New("build_production job must rebuild the selected static library from the immutable tag")
	}
	if err := requireCanonicalNamedRunStep(steps[5], "Check the generated diff", "git diff --check"); err != nil {
		return err
	}
	if err := requireProductionBuildStep(steps[6]); err != nil || workflowpolicy.RequireScalar(steps[6], "name", "Build the versioned native executable") != nil {
		return errors.New("build_production job must build the tag-bound executable")
	}
	if err := requireProductionPackageStep(steps[7]); err != nil || workflowpolicy.RequireScalar(steps[7], "name", "Package the fixed-name platform asset") != nil {
		return errors.New("build_production job must package the tag-bound executable")
	}
	return requireExactActionStep(steps[8], "verified release build artifact upload", uploadArtifactAction, map[string]string{
		"name":              "verified-release-${{ matrix.artifact }}",
		"path":              "dist/pixiv-cli_*",
		"if-no-files-found": "error",
		"retention-days":    "1",
	})
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
		if _, exists := workflowpolicy.MappingValue(step, key); exists {
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
	if err := workflowpolicy.RequireScalar(step, "shell", "bash"); err != nil {
		return fmt.Errorf("build quality gate %s must use shell bash", command)
	}
	if directory == "" {
		if _, exists := workflowpolicy.MappingValue(step, "working-directory"); exists {
			return fmt.Errorf("build quality gate %s must run from the repository root", command)
		}
		return nil
	}
	if err := workflowpolicy.RequireScalar(step, "working-directory", directory); err != nil {
		return fmt.Errorf("build quality gate %s must run from %s", command, directory)
	}
	return nil
}

func checkReleaseMatrix(matrix *yaml.Node) error {
	if err := requireOnlyMappingKeys(matrix, "include"); err != nil {
		return errors.New("build matrix must contain only the six release targets")
	}
	include, ok := workflowpolicy.MappingValue(matrix, "include")
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
		fields := make([]string, 0, 7)
		for _, key := range []string{"runner", "goos", "goarch", "rust_target", "rust_toolchain", "artifact", "cc"} {
			value, ok := workflowpolicy.MappingValue(entry, key)
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
