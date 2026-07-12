// Command releaseworkflow 检查发布 workflow 的结构化安全与质量门禁。
package main

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// actionReferencePattern 只接受远端 action 的不可变完整对象 ID，避免可移动 tag 改写发布供应链。
var actionReferencePattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+@[0-9a-f]{40}$`)

// 解析后的 YAML scalar 保留 GitHub expression。任一 `${{ ... }}` 内独立的 secrets context
// （包括 bare、toJSON(secrets)、dot 和 bracket）都视为凭据引用，不能依赖具体访问语法。
var secretReferencePattern = regexp.MustCompile(`(?is)\$\{\{[^}]*\bsecrets\b[^}]*\}\}`)

// releaseMatrixTargets 将 runner、Go 平台、Rust target 和 release asset 名称绑为同一集合，
// 防止任一字段的局部改动让六平台发布遗漏或错配。
var releaseMatrixTargets = map[string]struct{}{
	"macos-15-intel|darwin|amd64|x86_64-apple-darwin|darwin-amd64":       {},
	"macos-15|darwin|arm64|aarch64-apple-darwin|darwin-arm64":            {},
	"ubuntu-24.04|linux|amd64|x86_64-unknown-linux-gnu|linux-amd64":      {},
	"ubuntu-24.04-arm|linux|arm64|aarch64-unknown-linux-gnu|linux-arm64": {},
	"windows-2025|windows|amd64|x86_64-pc-windows-msvc|windows-amd64":    {},
	"windows-11-arm|windows|arm64|aarch64-pc-windows-msvc|windows-arm64": {},
}

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
	return checkWorkflow(body)
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
	if err := requireOnlyMappingKeys(jobs, "validate", "build", "verify_release_source", "publish"); err != nil {
		return fmt.Errorf("workflow jobs: %w", err)
	}
	if err := checkActionReferences(root); err != nil {
		return err
	}
	validate, ok := mappingValue(jobs, "validate")
	if !ok || validate.Kind != yaml.MappingNode {
		return errors.New("workflow must have a validate job")
	}
	build, ok := mappingValue(jobs, "build")
	if !ok || build.Kind != yaml.MappingNode {
		return errors.New("workflow must have a build job")
	}
	verifyReleaseSource, ok := mappingValue(jobs, "verify_release_source")
	if !ok || verifyReleaseSource.Kind != yaml.MappingNode {
		return errors.New("workflow must have a verify_release_source job")
	}
	publish, ok := mappingValue(jobs, "publish")
	if !ok || publish.Kind != yaml.MappingNode {
		return errors.New("workflow must have a publish job")
	}
	if err := checkValidateJob(validate); err != nil {
		return err
	}
	if err := checkBuildJob(build); err != nil {
		return err
	}
	if err := checkVerifyReleaseSourceJob(verifyReleaseSource); err != nil {
		return err
	}
	signingIndex, publishSteps, err := checkPublishJob(publish)
	if err != nil {
		return err
	}
	if err := checkSigningSecretReachability(validate, build, verifyReleaseSource, publish, publishSteps, signingIndex); err != nil {
		return err
	}
	return nil
}

func checkTagTrigger(root *yaml.Node) error {
	on, ok := mappingValue(root, "on")
	if !ok || on.Kind != yaml.MappingNode {
		return errors.New("workflow must have an on mapping")
	}
	if err := requireOnlyMappingKeys(on, "push"); err != nil {
		return errors.New("on must contain only the push trigger")
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
	return nil
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
	if err := requireCredentialFreeCheckout(job, "validate job"); err != nil {
		return err
	}
	if !hasCommand(job, "", "go run ./scripts/releaseassets validate --version \"${GITHUB_REF_NAME#v}\"") {
		return errors.New("validate job must run releaseassets validate")
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
	if err := requireOnlyMappingKeys(job, "name", "needs", "runs-on", "permissions", "strategy", "steps"); err != nil {
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
	if err := requireCredentialFreeCheckout(job, "build job"); err != nil {
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
	for _, key := range []string{"defaults", "env"} {
		if _, exists := mappingValue(root, key); exists {
			return fmt.Errorf("workflow root must not declare %s", key)
		}
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
	for _, key := range []string{"env", "defaults"} {
		if _, exists := mappingValue(job, key); exists {
			return fmt.Errorf("%s must not declare %s", jobName, key)
		}
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
	include, ok := mappingValue(matrix, "include")
	if !ok || include.Kind != yaml.SequenceNode || len(include.Content) != len(releaseMatrixTargets) {
		return errors.New("build matrix must contain exactly the six release targets")
	}
	seen := make(map[string]struct{}, len(include.Content))
	for _, entry := range include.Content {
		if entry.Kind != yaml.MappingNode {
			return errors.New("build matrix must contain exactly the six release targets")
		}
		fields := make([]string, 0, 5)
		for _, key := range []string{"runner", "goos", "goarch", "rust_target", "artifact"} {
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
	if err := requireScalar(job, "needs", "build"); err != nil {
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
	ancestryIndex := -1
	for index, step := range steps {
		if hasStepCommand(step, "git merge-base --is-ancestor HEAD \"origin/$DEFAULT_BRANCH\"") {
			ancestryIndex = index
			if err := requireUnconditionalAncestryGate(step); err != nil {
				return err
			}
			if !hasStepCommand(step, "git show-ref --verify --quiet \"refs/remotes/origin/$DEFAULT_BRANCH\"") {
				return errors.New("verify_release_source default-branch ancestry gate must verify origin/$DEFAULT_BRANCH")
			}
			env, ok := mappingValue(step, "env")
			if !ok || requireOnlyMappingKeys(env, "DEFAULT_BRANCH") != nil || requireScalar(env, "DEFAULT_BRANCH", "${{ github.event.repository.default_branch }}") != nil {
				return errors.New("verify_release_source default-branch ancestry gate must derive DEFAULT_BRANCH from the release repository")
			}
			break
		}
	}
	if ancestryIndex < 0 {
		return errors.New("verify_release_source job must contain a default-branch ancestry gate")
	}
	if !hasTrustedCheckoutBefore(steps, ancestryIndex) {
		return errors.New("verify_release_source default-branch ancestry gate requires a full, credential-free checkout before it runs")
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
	signingIndex, signingStep := signingStepIndex(steps)
	if signingIndex < 0 {
		return 0, nil, errors.New("publish job must contain a signing-secret step")
	}
	if err := checkSigningStep(signingStep); err != nil {
		return 0, nil, err
	}
	for _, step := range steps {
		run, hasRun := mappingValue(step, "run")
		if hasRun && strings.Contains(run.Value, "${GITHUB_REF_NAME#v}") && strings.Contains(run.Value, "*-*") {
			return 0, nil, errors.New("publish job must not classify prereleases with a hyphen shell pattern")
		}
	}
	releaseStep, ok := rootStepWithRunFragment(job, "gh release create \"$GITHUB_REF_NAME\"")
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
	if err := requireRunFragments(releaseStep, "release publishing step", "${prerelease[@]}", "release/checksums.json", "gh release view \"$GITHUB_REF_NAME\"", "gh release edit \"$GITHUB_REF_NAME\" --draft=false"); err != nil {
		return 0, nil, err
	}
	return signingIndex, steps, nil
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
	channelCase := "case \"$(go run ./scripts/releaseassets channel --version \"${GITHUB_REF_NAME#v}\")\" in"
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
		"gh release create \"$GITHUB_REF_NAME\" \\",
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
	releaseCreateCommand := `gh release create "$GITHUB_REF_NAME" \`
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

func checkSigningSecretReachability(validate, build, verifyReleaseSource, publish *yaml.Node, publishSteps []*yaml.Node, signingIndex int) error {
	// release Environment 的 secret 只应在 verify_release_source 成功后才由 publish job 注入；
	// 非发布 job、publish 的 job 级字段和非签名 step 都不允许引用 secrets，防止同一 job
	// 启动即注入的 credential 在 shell gate 或其它步骤被提前访问。
	for _, job := range []*yaml.Node{validate, build, verifyReleaseSource} {
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

func hasTrustedCheckoutBefore(steps []*yaml.Node, before int) bool {
	for index := 0; index < before; index++ {
		uses, hasUses := mappingValue(steps[index], "uses")
		with, hasWith := mappingValue(steps[index], "with")
		if !hasUses || !hasWith || !strings.HasPrefix(uses.Value, "actions/checkout@") {
			continue
		}
		if _, conditional := mappingValue(steps[index], "if"); conditional {
			continue
		}
		if _, softFailure := mappingValue(steps[index], "continue-on-error"); softFailure {
			continue
		}
		if requireScalar(with, "fetch-depth", "0") == nil && requireScalar(with, "persist-credentials", "false") == nil {
			return true
		}
	}
	return false
}

func requireCredentialFreeCheckout(job *yaml.Node, jobName string) error {
	steps, err := jobSteps(job)
	if err != nil {
		return fmt.Errorf("%s must have exactly one credential-free checkout", jobName)
	}
	checkoutCount := 0
	for _, step := range steps {
		uses, hasUses := mappingValue(step, "uses")
		if !hasUses || uses.Kind != yaml.ScalarNode || !strings.HasPrefix(uses.Value, "actions/checkout@") {
			continue
		}
		checkoutCount++
		if _, exists := mappingValue(step, "if"); exists {
			return fmt.Errorf("%s checkout must not define if or continue-on-error", jobName)
		}
		if _, exists := mappingValue(step, "continue-on-error"); exists {
			return fmt.Errorf("%s checkout must not define if or continue-on-error", jobName)
		}
		with, ok := mappingValue(step, "with")
		if !ok || requireScalar(with, "persist-credentials", "false") != nil {
			return fmt.Errorf("%s checkout must set persist-credentials to false", jobName)
		}
	}
	if checkoutCount != 1 {
		return fmt.Errorf("%s must have exactly one credential-free checkout", jobName)
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
