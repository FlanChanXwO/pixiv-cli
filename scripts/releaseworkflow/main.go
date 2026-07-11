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
	if document.Kind != yaml.DocumentNode || len(document.Content) != 1 {
		return errors.New("workflow must contain exactly one YAML document")
	}
	root := document.Content[0]
	if root.Kind != yaml.MappingNode {
		return errors.New("workflow root must be a mapping")
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
	if err := requireOnlyMappingKeys(jobs, "validate", "build", "publish"); err != nil {
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
	ancestryStep, publishSteps, err := checkPublishJob(publish)
	if err != nil {
		return err
	}
	if err := checkSigningSecretReachability(jobs, publishSteps, ancestryStep); err != nil {
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
	if err := requireScalar(job, "runs-on", "ubuntu-24.04"); err != nil {
		return fmt.Errorf("validate job: %w", err)
	}
	if _, exists := mappingValue(job, "needs"); exists {
		return errors.New("validate job must not depend on another job")
	}
	if err := requireContentsPermission(job, "read"); err != nil {
		return fmt.Errorf("validate job: %w", err)
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
	if err := requireScalar(job, "needs", "validate"); err != nil {
		return fmt.Errorf("build job: %w", err)
	}
	if err := requireScalar(job, "runs-on", "${{ matrix.runner }}"); err != nil {
		return fmt.Errorf("build job: %w", err)
	}
	if err := requireContentsPermission(job, "read"); err != nil {
		return fmt.Errorf("build job: %w", err)
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
		if !hasCommand(job, gate.workingDirectory, gate.command) {
			return fmt.Errorf("build must run %s", gate.command)
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

func checkPublishJob(job *yaml.Node) (int, []*yaml.Node, error) {
	if err := requireScalar(job, "needs", "build"); err != nil {
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
	ancestryIndex := -1
	for index, step := range steps {
		if hasStepCommand(step, "git merge-base --is-ancestor HEAD \"origin/$DEFAULT_BRANCH\"") {
			ancestryIndex = index
			if !hasStepCommand(step, "git show-ref --verify --quiet \"refs/remotes/origin/$DEFAULT_BRANCH\"") {
				return 0, nil, errors.New("default-branch ancestry gate must verify origin/$DEFAULT_BRANCH")
			}
			env, ok := mappingValue(step, "env")
			if !ok || requireScalar(env, "DEFAULT_BRANCH", "${{ github.event.repository.default_branch }}") != nil {
				return 0, nil, errors.New("default-branch ancestry gate must derive DEFAULT_BRANCH from the release repository")
			}
			break
		}
	}
	if ancestryIndex < 0 {
		return 0, nil, errors.New("publish job must contain a default-branch ancestry gate")
	}
	if !hasTrustedCheckoutBefore(steps, ancestryIndex) {
		return 0, nil, errors.New("default-branch ancestry gate requires a full, credential-free checkout before it runs")
	}
	signingIndex, signingStep := signingStepIndex(steps)
	if signingIndex < 0 {
		return 0, nil, errors.New("publish job must contain a signing-secret step")
	}
	if signingIndex <= ancestryIndex {
		return 0, nil, errors.New("signing secret is reachable before the default-branch ancestry gate")
	}
	if err := checkSigningStep(signingStep); err != nil {
		return 0, nil, err
	}
	if !hasCommand(job, "", "channel=$(go run ./scripts/releaseassets channel --version \"${GITHUB_REF_NAME#v}\")") {
		return 0, nil, errors.New("publish job must classify the tag with releaseassets channel")
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
	if err := requireRunFragments(releaseStep, "release publishing step", "${prerelease[@]}", "release/checksums.json", "gh release view \"$GITHUB_REF_NAME\"", "gh release edit \"$GITHUB_REF_NAME\" --draft=false"); err != nil {
		return 0, nil, err
	}
	return ancestryIndex, steps, nil
}

func checkSigningSecretReachability(jobs *yaml.Node, publishSteps []*yaml.Node, ancestryIndex int) error {
	// GitHub 只会在 job 启动时注入 environment secret；因此除 publish 在信任 gate 后的步骤外，
	// 任何 `${{ secrets.* }}` 引用都意味着未经默认分支验证就可能触及凭据。
	for index := 0; index+1 < len(jobs.Content); index += 2 {
		name, job := jobs.Content[index].Value, jobs.Content[index+1]
		if name != "publish" && containsSigningSecretReference(job) {
			return errors.New("signing secret is reachable before the default-branch ancestry gate")
		}
	}
	for index, step := range publishSteps {
		if index <= ancestryIndex && containsSigningSecretReference(step) {
			return errors.New("signing secret is reachable before the default-branch ancestry gate")
		}
	}
	return nil
}

func checkSigningStep(step *yaml.Node) error {
	env, ok := mappingValue(step, "env")
	if !ok || env.Kind != yaml.MappingNode {
		return errors.New("signing-secret step must declare its secret environment")
	}
	if err := requireScalar(env, "RELEASE_SIGNING_PRIVATE_KEY", "${{ secrets.RELEASE_SIGNING_PRIVATE_KEY }}"); err != nil {
		return errors.New("signing-secret step must use the protected private-key secret")
	}
	if err := requireScalar(env, "RELEASE_SIGNING_KEY_ID", "${{ secrets.RELEASE_SIGNING_KEY_ID }}"); err != nil {
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

func containsSigningSecretReference(node *yaml.Node) bool {
	if node == nil {
		return false
	}
	if node.Kind == yaml.ScalarNode && strings.Contains(node.Value, "${{ secrets.") {
		return true
	}
	for _, child := range node.Content {
		if containsSigningSecretReference(child) {
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
		if requireScalar(with, "fetch-depth", "0") == nil && requireScalar(with, "persist-credentials", "false") == nil {
			return true
		}
	}
	return false
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
	run, hasRun := mappingValue(step, "run")
	if !hasRun || run.Kind != yaml.ScalarNode {
		return fmt.Errorf("%s must have a run command", context)
	}
	for _, fragment := range fragments {
		if !strings.Contains(run.Value, fragment) {
			return fmt.Errorf("%s must contain %s", context, fragment)
		}
	}
	return nil
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
