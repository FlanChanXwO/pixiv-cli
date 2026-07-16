package main

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/scripts/internal/workflowpolicy"
	"gopkg.in/yaml.v3"
)

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
	if err := workflowpolicy.RequireScalar(job, "needs", "build_production"); err != nil {
		return fmt.Errorf("verify_release_source job: %w", err)
	}
	if err := workflowpolicy.RequireScalar(job, "runs-on", "ubuntu-24.04"); err != nil {
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
	env, ok := workflowpolicy.MappingValue(ancestryGate, "env")
	if !ok || requireOnlyMappingKeys(env, "DEFAULT_BRANCH") != nil || workflowpolicy.RequireScalar(env, "DEFAULT_BRANCH", "${{ github.event.repository.default_branch }}") != nil {
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
	if err := workflowpolicy.RequireScalar(step, "uses", canonicalCheckoutAction); err != nil {
		return fmt.Errorf("%s must use the canonical checkout", jobName)
	}
	with, ok := workflowpolicy.MappingValue(step, "with")
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
		if err := workflowpolicy.RequireScalar(with, requirement.key, requirement.value); err != nil {
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
	if err := workflowpolicy.RequireScalar(job, "needs", "verify_release_source"); err != nil {
		return 0, nil, fmt.Errorf("publish job: %w", err)
	}
	if err := workflowpolicy.RequireScalar(job, "runs-on", "ubuntu-24.04"); err != nil {
		return 0, nil, fmt.Errorf("publish job: %w", err)
	}
	if err := workflowpolicy.RequireScalar(job, "environment", "release"); err != nil {
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
		run, hasRun := workflowpolicy.MappingValue(step, "run")
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

func requireUnconditionalAncestryGate(step *yaml.Node) error {
	for _, key := range []string{"continue-on-error", "if"} {
		if _, exists := workflowpolicy.MappingValue(step, key); exists {
			return errors.New("verify_release_source default-branch ancestry gate must not define if or continue-on-error")
		}
	}
	if err := requireOnlyMappingKeys(step, "name", "shell", "env", "run"); err != nil {
		return errors.New("verify_release_source default-branch ancestry gate must be a direct bash step")
	}
	if err := workflowpolicy.RequireScalar(step, "shell", "bash"); err != nil {
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
		if workflowpolicy.ContainsSecretReference(job) {
			return errors.New("non-release job must not reference secrets")
		}
	}
	for index := 0; index+1 < len(publish.Content); index += 2 {
		if publish.Content[index].Value != "steps" && workflowpolicy.ContainsSecretReference(publish.Content[index+1]) {
			return errors.New("publish job must not reference secrets outside its signing metadata step")
		}
	}
	for index, step := range publishSteps {
		if index != signingIndex && workflowpolicy.ContainsSecretReference(step) {
			return errors.New("publish job must not reference secrets outside its signing metadata step")
		}
		if index == signingIndex && containsSigningSecretReferenceOutsideEnvironment(step) {
			return errors.New("signing metadata step must reference secrets only through its expected environment")
		}
	}
	return nil
}

func checkSigningStep(step *yaml.Node) error {
	env, ok := workflowpolicy.MappingValue(step, "env")
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
	run, _ := workflowpolicy.MappingValue(step, "run")
	if err := requireRunFragments(step, "signing-secret step", "trap 'rm -f \"$key_path\"'", "go run ./scripts/releaseassets finalize", "--input-dir dist", "--output-dir release", "--private-key \"$key_path\""); err != nil {
		return err
	}
	if strings.Contains(run.Value, "set -x") || strings.Contains(run.Value, "echo $RELEASE_SIGNING") {
		return errors.New("signing-secret step must not print signing secret values")
	}
	return nil
}

func requireExpectedSigningSecret(env *yaml.Node, key string) error {
	value, ok := workflowpolicy.MappingValue(env, key)
	if !ok || value.Kind != yaml.ScalarNode || !expectedSigningSecretExpression(key).MatchString(value.Value) {
		return fmt.Errorf("%s must be its exact signing secret expression", key)
	}
	return nil
}

func expectedSigningSecretExpression(key string) *regexp.Regexp {
	quotedKey := regexp.QuoteMeta(key)
	return regexp.MustCompile(`^\$\{\{\s*secrets\s*(?:\.\s*` + quotedKey + `|\[\s*['"]` + quotedKey + `['"]\s*\])\s*\}\}$`)
}

func containsSigningSecretReferenceOutsideEnvironment(step *yaml.Node) bool {
	if step == nil || step.Kind != yaml.MappingNode {
		return workflowpolicy.ContainsSecretReference(step)
	}
	for index := 0; index+1 < len(step.Content); index += 2 {
		if step.Content[index].Value == "env" {
			continue
		}
		if workflowpolicy.ContainsSecretReference(step.Content[index+1]) {
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
		uses, hasUses := workflowpolicy.MappingValue(step, "uses")
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
		env, ok := workflowpolicy.MappingValue(step, "env")
		if !ok {
			continue
		}
		if _, exists := workflowpolicy.MappingValue(env, "RELEASE_SIGNING_PRIVATE_KEY"); exists {
			return index, step
		}
	}
	return -1, nil
}
