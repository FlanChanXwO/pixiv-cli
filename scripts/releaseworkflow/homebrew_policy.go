package main

import (
	"errors"
	"fmt"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/scripts/internal/workflowpolicy"
	"gopkg.in/yaml.v3"
)

// homebrewMatrixTargets 绑定四个 Homebrew 验证 runner 与其实际平台，避免仅在单一
// 架构验证 staging formula 后就向公开 tap 推送。
var homebrewMatrixTargets = map[string]struct{}{
	"macos-15-intel|darwin|amd64":  {},
	"macos-15|darwin|arm64":        {},
	"ubuntu-22.04|linux|amd64":     {},
	"ubuntu-22.04-arm|linux|arm64": {},
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
	if err := workflowpolicy.RequireScalar(job, "needs", "publish"); err != nil {
		return fmt.Errorf("render_homebrew_formula job: %w", err)
	}
	if err := workflowpolicy.RequireScalar(job, "runs-on", "ubuntu-24.04"); err != nil {
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
formula_name=$(cat staging-formula/formula-name)
case "$formula_name" in
pixiv-cli|pixiv-cli-beta) ;;
*) exit 1 ;;
esac
test "$(find staging-formula -maxdepth 1 -type f -print | LC_ALL=C sort)" = "$(printf '%s\n%s\n' staging-formula/formula-name "staging-formula/$formula_name.rb" | LC_ALL=C sort)"
if [ '${{ matrix.os }}' = linux ]; then
staging_dir="$(pwd)/staging-formula"
case "$staging_dir" in /*) ;; *) exit 1 ;; esac
docker run --rm \
--mount "type=bind,src=$staging_dir,dst=/staging-formula,readonly" \
--env RELEASE_TAG \
--env HOMEBREW_NO_AUTO_UPDATE=1 \
--env HOMEBREW_NO_ENV_HINTS=1 \
--entrypoint bash \
homebrew/brew@sha256:b0072bfdebf5934ae24b93b44a1928a88057399b3283ffa0177bb86084fdedfd \
-euo pipefail -c '
formula_name=$(cat /staging-formula/formula-name)
case "$formula_name" in
pixiv-cli|pixiv-cli-beta) ;;
*) exit 1 ;;
esac
test "$(find /staging-formula -maxdepth 1 -type f -print | LC_ALL=C sort)" = "$(printf "%s\\n%s\\n" /staging-formula/formula-name "/staging-formula/$formula_name.rb" | LC_ALL=C sort)"
staging_tap=pixiv-cli-release/staging
tap_dir="$(brew --repository)/Library/Taps/pixiv-cli-release/homebrew-staging"
brew tap-new "$staging_tap" --no-git
brew trust --tap "$staging_tap"
cp "/staging-formula/$formula_name.rb" "$tap_dir/Formula/$formula_name.rb"
brew install --formula "$staging_tap/$formula_name"
pixiv version --json | python3 -c "import json, sys; actual = json.load(sys.stdin)[\"version\"]; expected = sys.argv[1]; assert actual == expected, f\"version {actual!r} != {expected!r}\"" "$RELEASE_TAG"
'
else
staging_tap=pixiv-cli-release/staging
tap_dir="$(brew --repository)/Library/Taps/pixiv-cli-release/homebrew-staging"
brew tap-new "$staging_tap" --no-git
brew trust --tap "$staging_tap"
cp "staging-formula/$formula_name.rb" "$tap_dir/Formula/$formula_name.rb"
brew install --formula "$staging_tap/$formula_name"
pixiv version --json | python3 -c 'import json, sys; actual = json.load(sys.stdin)["version"]; expected = sys.argv[1]; assert actual == expected, f"version {actual!r} != {expected!r}"' "$RELEASE_TAG"
fi
`

	if err := requireRequiredJobExecution(job, "verify_homebrew_formula job"); err != nil {
		return err
	}
	if err := requireNoEnvironment(job, "verify_homebrew_formula job"); err != nil {
		return err
	}
	if err := requireOnlyMappingKeys(job, "name", "needs", "runs-on", "permissions", "strategy", "steps"); err != nil {
		return fmt.Errorf("verify_homebrew_formula job: %w", err)
	}
	if err := workflowpolicy.RequireScalar(job, "needs", "render_homebrew_formula"); err != nil {
		return fmt.Errorf("verify_homebrew_formula job: %w", err)
	}
	if err := workflowpolicy.RequireScalar(job, "runs-on", "${{ matrix.runner }}"); err != nil {
		return fmt.Errorf("verify_homebrew_formula job: %w", err)
	}
	if err := requireContentsPermission(job, "read"); err != nil {
		return fmt.Errorf("verify_homebrew_formula job: %w", err)
	}
	strategy, ok := workflowpolicy.MappingValue(job, "strategy")
	if !ok || requireOnlyMappingKeys(strategy, "fail-fast", "matrix") != nil || workflowpolicy.RequireScalar(strategy, "fail-fast", "false") != nil {
		return errors.New("verify_homebrew_formula strategy must use fail-fast false and the exact four-target matrix")
	}
	matrix, _ := workflowpolicy.MappingValue(strategy, "matrix")
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
	include, ok := workflowpolicy.MappingValue(matrix, "include")
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
			value, ok := workflowpolicy.MappingValue(entry, key)
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
	if err := workflowpolicy.RequireScalar(job, "needs", "verify_homebrew_formula"); err != nil {
		return fmt.Errorf("deploy_homebrew_tap job: %w", err)
	}
	if err := workflowpolicy.RequireScalar(job, "runs-on", "ubuntu-24.04"); err != nil {
		return fmt.Errorf("deploy_homebrew_tap job: %w", err)
	}
	if err := workflowpolicy.RequireScalar(job, "environment", "release"); err != nil {
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

func checkHomebrewSecretReachability(render, verify, deploy *yaml.Node) error {
	if workflowpolicy.ContainsSecretReference(render) || workflowpolicy.ContainsSecretReference(verify) {
		return errors.New("Homebrew render and install jobs must not reference secrets")
	}
	for index := 0; index+1 < len(deploy.Content); index += 2 {
		if deploy.Content[index].Value != "steps" && workflowpolicy.ContainsSecretReference(deploy.Content[index+1]) {
			return errors.New("deploy_homebrew_tap job must not reference secrets outside its final push step")
		}
	}
	steps, err := jobSteps(deploy)
	if err != nil {
		return nil
	}
	for index, step := range steps {
		if index != len(steps)-1 && workflowpolicy.ContainsSecretReference(step) {
			return errors.New("deploy_homebrew_tap job must not reference secrets outside its final push step")
		}
		if index == len(steps)-1 && containsSigningSecretReferenceOutsideEnvironment(step) {
			return errors.New("tap final push step must reference its secret only through the expected environment")
		}
	}
	if len(steps) == 0 {
		return nil
	}
	env, ok := workflowpolicy.MappingValue(steps[len(steps)-1], "env")
	if !ok || requireOnlyMappingKeys(env, "HOMEBREW_TAP_DEPLOY_KEY") != nil {
		return errors.New("tap final push step must declare only HOMEBREW_TAP_DEPLOY_KEY")
	}
	value, ok := workflowpolicy.MappingValue(env, "HOMEBREW_TAP_DEPLOY_KEY")
	if !ok || value.Kind != yaml.ScalarNode || !expectedSigningSecretExpression("HOMEBREW_TAP_DEPLOY_KEY").MatchString(value.Value) {
		return errors.New("tap final push step must use the protected HOMEBREW_TAP_DEPLOY_KEY secret")
	}
	return nil
}
