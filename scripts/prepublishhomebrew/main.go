// Command prepublishhomebrew 检查 Homebrew 发布前验证与受保护恢复 workflow 的安全边界。
package main

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/scripts/internal/workflowpolicy"
	"gopkg.in/yaml.v3"
)

var actionReferencePattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+@[0-9a-f]{40}$`)

var nativeTargets = map[string]struct{}{
	"macos-15-intel|darwin|amd64":  {},
	"macos-15|darwin|arm64":        {},
	"ubuntu-22.04|linux|amd64":     {},
	"ubuntu-22.04-arm|linux|arm64": {},
}

// 四段 shell 都是只读演练的完整 allowlist。逐字绑定而不是检查片段，避免通过
// 在既有步骤尾部追加 Release、tap 或网络写入命令绕过 workflow policy。
const validateReleaseRun = `set -euo pipefail
test "$GITHUB_REF" = "refs/heads/$DEFAULT_BRANCH"
go run ./scripts/releaseassets validate --version "${RELEASE_TAG#v}"
case "$RELEASE_TAG" in
  v[0-9]*) ;;
  *) printf '%s\n' 'release tag must start with v and a digit' >&2; exit 1 ;;
esac
gh api "repos/$GITHUB_REPOSITORY/releases/tags/$RELEASE_TAG" > release.json
python3 - "$RELEASE_TAG" release.json <<'PY'
import json
import pathlib
import sys

tag = sys.argv[1]
release = json.loads(pathlib.Path(sys.argv[2]).read_text(encoding="utf-8"))
if release.get("tag_name") != tag:
    raise SystemExit("release tag does not match the requested tag")
if release.get("draft") or release.get("prerelease") or not release.get("published_at"):
    raise SystemExit("requested release must already be public and stable")
PY
`

const downloadChecksumsRun = `set -euo pipefail
mkdir -p verified-release
gh release download "$RELEASE_TAG" --pattern checksums.txt --dir verified-release
test "$(find verified-release -maxdepth 1 -type f -print | LC_ALL=C sort)" = "verified-release/checksums.txt"
`

const renderStableFormulaRun = `set -euo pipefail
version="${RELEASE_TAG#v}"
test "$(go run ./scripts/releaseassets channel --version "$version")" = stable
mkdir -p staging-formula
go run ./scripts/homebrewformula render \
  --formula pixiv-cli \
  --version "$version" \
  --checksums verified-release/checksums.txt \
  --output staging-formula/pixiv-cli.rb
printf '%s\n' pixiv-cli > staging-formula/formula-name
`

const verifyStagingFormulaRun = `set -euo pipefail
formula_name=$(cat staging-formula/formula-name)
test "$formula_name" = pixiv-cli
test "$(find staging-formula -maxdepth 1 -type f -print | LC_ALL=C sort)" = "$(printf '%s\n%s\n' staging-formula/formula-name staging-formula/pixiv-cli.rb | LC_ALL=C sort)"
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
      test "$formula_name" = pixiv-cli
      test "$(find /staging-formula -maxdepth 1 -type f -print | LC_ALL=C sort)" = "$(printf "%s\\n%s\\n" /staging-formula/formula-name /staging-formula/pixiv-cli.rb | LC_ALL=C sort)"
      staging_tap=pixiv-cli-release/staging
      tap_dir="$(brew --repository)/Library/Taps/pixiv-cli-release/homebrew-staging"
      brew tap-new "$staging_tap" --no-git
      cp /staging-formula/pixiv-cli.rb "$tap_dir/Formula/pixiv-cli.rb"
      brew install --formula "$staging_tap/$formula_name"
      pixiv version --json | brew ruby -rjson -e "actual = JSON.parse(STDIN.read).fetch(\"version\"); expected = ARGV.fetch(0); abort(\"version #{actual.inspect} != #{expected.inspect}\") unless actual == expected" "$RELEASE_TAG"
    '
else
  staging_tap=pixiv-cli-release/staging
  tap_dir="$(brew --repository)/Library/Taps/pixiv-cli-release/homebrew-staging"
  brew tap-new "$staging_tap" --no-git
  brew trust --tap "$staging_tap"
  cp staging-formula/pixiv-cli.rb "$tap_dir/Formula/pixiv-cli.rb"
  brew install --formula "$staging_tap/$formula_name"
  pixiv version --json | python3 -c 'import json, sys; actual = json.load(sys.stdin)["version"]; expected = sys.argv[1]; assert actual == expected, f"version {actual!r} != {expected!r}"' "$RELEASE_TAG"
fi
`

// deploy 仅可消费已由四平台 install gate 验证的 artifact。它允许同一份公式已经
// 存在，以便在网络抖动后安全重跑；除此之外不允许暂存任何 tap 文件。
const stageVerifiedFormulaRun = `set -euo pipefail
formula_name=$(cat staging-formula/formula-name)
test "$formula_name" = pixiv-cli
test "$(find staging-formula -maxdepth 1 -type f -print | LC_ALL=C sort)" = "$(printf '%s\n%s\n' staging-formula/formula-name staging-formula/pixiv-cli.rb | LC_ALL=C sort)"
tap_dir="$RUNNER_TEMP/homebrew-tap"
git clone https://github.com/FlanChanXwO/homebrew-tap.git "$tap_dir"
git -C "$tap_dir" config user.name github-actions[bot]
git -C "$tap_dir" config user.email 41898282+github-actions[bot]@users.noreply.github.com
mkdir -p "$tap_dir/Formula"
cp staging-formula/pixiv-cli.rb "$tap_dir/Formula/pixiv-cli.rb"
git -C "$tap_dir" add -- Formula/pixiv-cli.rb
staged=$(git -C "$tap_dir" diff --cached --name-only)
test "$staged" = "" || test "$staged" = Formula/pixiv-cli.rb
test -z "$(git -C "$tap_dir" status --porcelain | sed -n '\|^?? |p')"
if [ -n "$staged" ]; then
  git -C "$tap_dir" commit -m "${RELEASE_TAG}: update pixiv-cli formula"
fi
`

const pushFormulaRun = `set -eu
umask 077
key_path="$RUNNER_TEMP/homebrew-tap-deploy-key"
trap 'rm -f "$key_path"' EXIT HUP INT TERM
test -n "$HOMEBREW_TAP_DEPLOY_KEY"
printf '%s\n' "$HOMEBREW_TAP_DEPLOY_KEY" > "$key_path"
chmod 600 "$key_path"
tap_dir="$RUNNER_TEMP/homebrew-tap"
git -C "$tap_dir" remote set-url origin git@github.com:FlanChanXwO/homebrew-tap.git
GIT_SSH_COMMAND="ssh -i $key_path -o IdentitiesOnly=yes -o StrictHostKeyChecking=yes -o UserKnownHostsFile=$GITHUB_WORKSPACE/templates/homebrew/github.com-known-hosts" \
  git -C "$tap_dir" push origin HEAD:main
`

func main() {
	if len(os.Args) != 3 || os.Args[1] != "--workflow" {
		fmt.Fprintln(os.Stderr, "prepublish Homebrew workflow policy: usage: prepublishhomebrew --workflow PATH")
		os.Exit(1)
	}
	body, err := os.ReadFile(os.Args[2])
	if err != nil {
		fmt.Fprintf(os.Stderr, "prepublish Homebrew workflow policy: read workflow: %v\n", err)
		os.Exit(1)
	}
	if err := checkWorkflow(body); err != nil {
		fmt.Fprintf(os.Stderr, "prepublish Homebrew workflow policy: %v\n", err)
		os.Exit(1)
	}
}

func checkWorkflow(body []byte) error {
	var document yaml.Node
	if err := yaml.Unmarshal(body, &document); err != nil {
		return fmt.Errorf("parse YAML: %w", err)
	}
	if err := workflowpolicy.RejectAmbiguousYAML(&document); err != nil {
		return err
	}
	if document.Kind != yaml.DocumentNode || len(document.Content) != 1 {
		return errors.New("workflow must contain one document")
	}
	root := document.Content[0]
	if !exactKeys(root, "name", "on", "env", "permissions", "jobs") || scalar(root, "name") != "Homebrew prepublish verification and recovery" {
		return errors.New("workflow root is not the fixed Homebrew verification and recovery shape")
	}
	if err := checkDispatch(root); err != nil {
		return err
	}
	if !exactStringMap(value(root, "env"), map[string]string{"RELEASE_TAG": "${{ inputs.release_tag }}"}) || !contentsRead(root) {
		return errors.New("workflow must bind only the required release_tag with contents read")
	}
	// 唯一 secret 只能留给经过 Environment 审批的最终 SSH push step。
	if strings.Count(string(body), "${{ secrets.") != 1 {
		return errors.New("workflow must reference exactly one protected deploy secret")
	}
	if err := checkActions(root); err != nil {
		return err
	}
	jobs := value(root, "jobs")
	if !exactKeys(jobs, "validate_existing_release", "render_stable_formula", "verify_homebrew_formula", "deploy_homebrew_tap") {
		return errors.New("workflow must contain only validation, render, native verify, and protected deploy jobs")
	}
	if err := checkValidate(value(jobs, "validate_existing_release")); err != nil {
		return err
	}
	if err := checkRender(value(jobs, "render_stable_formula")); err != nil {
		return err
	}
	if err := checkVerify(value(jobs, "verify_homebrew_formula")); err != nil {
		return err
	}
	return checkDeploy(value(jobs, "deploy_homebrew_tap"))
}

func checkDispatch(root *yaml.Node) error {
	on := value(root, "on")
	dispatch := value(on, "workflow_dispatch")
	inputs := value(dispatch, "inputs")
	releaseTag := value(inputs, "release_tag")
	deploy := value(inputs, "deploy")
	if !exactKeys(on, "workflow_dispatch") || !exactKeys(dispatch, "inputs") || !exactKeys(inputs, "release_tag", "deploy") ||
		!exactKeys(releaseTag, "description", "required", "type") || scalar(releaseTag, "description") != "Existing public stable Release tag to verify or recover" || scalar(releaseTag, "type") != "string" || !boolValue(releaseTag, "required", true) ||
		!exactKeys(deploy, "description", "required", "default", "type") || scalar(deploy, "description") != "Deploy the formula only after the four native checks pass" || scalar(deploy, "type") != "boolean" || !boolValue(deploy, "required", false) || !boolValue(deploy, "default", false) {
		return errors.New("workflow_dispatch must require release_tag and default deploy to false")
	}
	return nil
}

func checkValidate(job *yaml.Node) error {
	if !jobShape(job, "Validate an existing public stable Release", "", false) {
		return errors.New("validate job must be an unprivileged fixed Linux job")
	}
	steps := stepsOf(job)
	if len(steps) != 3 || !checkoutStep(steps[0]) || !goSetupStep(steps[1]) {
		return errors.New("validate job must use the fixed checkout and Go setup")
	}
	if !canonicalRunStep(steps[2], "Validate the manual tag and the public Release", validateReleaseRun, map[string]string{
		"DEFAULT_BRANCH": "${{ github.event.repository.default_branch }}",
		"GH_TOKEN":       "${{ github.token }}",
	}) {
		return errors.New("validate job must use the canonical read-only release check")
	}
	return nil
}

func checkRender(job *yaml.Node) error {
	if !jobShape(job, "Render a stable formula from Release checksums", "validate_existing_release", true) {
		return errors.New("render job must be an unprivileged fixed Linux job")
	}
	steps := stepsOf(job)
	if len(steps) != 6 || !checkoutStep(steps[0]) || !goSetupStep(steps[1]) {
		return errors.New("render job must use the fixed checkout and Go setup")
	}
	if !canonicalRunStep(steps[2], "Download only the public Release checksums", downloadChecksumsRun, map[string]string{"GH_TOKEN": "${{ github.token }}"}) ||
		!canonicalRunStep(steps[3], "Render exactly the stable formula", renderStableFormulaRun, nil) ||
		!artifactStep(steps[4], "staging-homebrew-formula", "staging-formula") ||
		!artifactStep(steps[5], "verified-release-checksums", "verified-release/checksums.txt") {
		return errors.New("render job must download Release checksums and render only pixiv-cli")
	}
	return nil
}

func checkVerify(job *yaml.Node) error {
	if !exactKeys(job, "name", "needs", "runs-on", "permissions", "strategy", "steps") ||
		scalar(job, "name") != "Verify Homebrew on ${{ matrix.os }}/${{ matrix.arch }}" ||
		scalar(job, "needs") != "render_stable_formula" || scalar(job, "runs-on") != "${{ matrix.runner }}" ||
		!contentsRead(job) || !nativeMatrix(value(job, "strategy")) {
		return errors.New("verify job must retain the exact four native targets")
	}
	steps := stepsOf(job)
	if len(steps) != 2 || !exactKeys(steps[0], "uses", "with") ||
		!strings.HasPrefix(scalar(steps[0], "uses"), "actions/download-artifact@") ||
		scalar(value(steps[0], "with"), "name") != "staging-homebrew-formula" ||
		scalar(value(steps[0], "with"), "path") != "staging-formula" {
		return errors.New("verify job must download only the staging formula")
	}
	if !canonicalRunStep(steps[1], "Install the local staging formula and verify its version", verifyStagingFormulaRun, nil) {
		return errors.New("verify job must use the canonical Linux and macOS Homebrew install paths")
	}
	return nil
}

func checkDeploy(job *yaml.Node) error {
	if !exactKeys(job, "name", "needs", "if", "runs-on", "environment", "permissions", "steps") ||
		scalar(job, "name") != "Deploy the verified Homebrew formula" || scalar(job, "needs") != "verify_homebrew_formula" ||
		scalar(job, "if") != "inputs.deploy == true" || scalar(job, "runs-on") != "ubuntu-24.04" ||
		scalar(job, "environment") != "release" || !contentsRead(job) {
		return errors.New("deploy job must be the protected post-matrix recovery gate")
	}
	steps := stepsOf(job)
	if len(steps) != 4 || !checkoutStep(steps[0]) || !exactKeys(steps[1], "uses", "with") ||
		scalar(steps[1], "uses") != "actions/download-artifact@d3f86a106a0bac45b974a628896c90dbdf5c8093" ||
		!exactStringMap(value(steps[1], "with"), map[string]string{"name": "staging-homebrew-formula", "path": "staging-formula"}) ||
		!canonicalRunStep(steps[2], "Stage exactly one verified formula", stageVerifiedFormulaRun, nil) ||
		!canonicalRunStep(steps[3], "Push with the release Environment deploy key", pushFormulaRun, map[string]string{"HOMEBREW_TAP_DEPLOY_KEY": "${{ secrets.HOMEBREW_TAP_DEPLOY_KEY }}"}) {
		return errors.New("deploy job must use the canonical protected formula recovery steps")
	}
	return nil
}

func jobShape(job *yaml.Node, name, needs string, hasNeeds bool) bool {
	keys := []string{"name", "runs-on", "permissions", "steps"}
	if hasNeeds {
		keys = append(keys, "needs")
	}
	return exactKeys(job, keys...) && scalar(job, "name") == name && scalar(job, "runs-on") == "ubuntu-24.04" && (!hasNeeds || scalar(job, "needs") == needs) && contentsRead(job)
}

func nativeMatrix(strategy *yaml.Node) bool {
	if !exactKeys(strategy, "fail-fast", "matrix") || !boolValue(strategy, "fail-fast", false) {
		return false
	}
	matrix := value(strategy, "matrix")
	include := value(matrix, "include")
	if !exactKeys(matrix, "include") || include == nil || include.Kind != yaml.SequenceNode || len(include.Content) != len(nativeTargets) {
		return false
	}
	seen := map[string]struct{}{}
	for _, target := range include.Content {
		if !exactKeys(target, "runner", "os", "arch") {
			return false
		}
		identity := strings.Join([]string{scalar(target, "runner"), scalar(target, "os"), scalar(target, "arch")}, "|")
		if _, ok := nativeTargets[identity]; !ok {
			return false
		}
		if _, duplicate := seen[identity]; duplicate {
			return false
		}
		seen[identity] = struct{}{}
	}
	return true
}

func checkActions(node *yaml.Node) error {
	if node == nil {
		return errors.New("workflow contains nil node")
	}
	if node.Kind == yaml.MappingNode {
		if uses := value(node, "uses"); uses != nil && (uses.Kind != yaml.ScalarNode || !actionReferencePattern.MatchString(uses.Value)) {
			return errors.New("workflow actions must use immutable full SHA references")
		}
	}
	for _, child := range node.Content {
		if err := checkActions(child); err != nil {
			return err
		}
	}
	return nil
}

func checkoutStep(step *yaml.Node) bool {
	return actionStep(step, "actions/checkout@34e114876b0b11c390a56381ad16ebd13914f8d5", "persist-credentials", "false") && scalar(value(step, "with"), "ref") == "${{ github.sha }}"
}

func goSetupStep(step *yaml.Node) bool {
	return actionStep(step, "actions/setup-go@40f1582b2485089dde7abd97c1529aa768e1baff", "go-version", "1.26.3")
}

func actionStep(step *yaml.Node, action, key, want string) bool {
	return exactKeys(step, "uses", "with") && scalar(step, "uses") == action && scalar(value(step, "with"), key) == want
}

func artifactStep(step *yaml.Node, name, path string) bool {
	return exactKeys(step, "uses", "with") &&
		scalar(step, "uses") == "actions/upload-artifact@ea165f8d65b6e75b540449e92b4886f43607fa02" &&
		exactStringMap(value(step, "with"), map[string]string{
			"name": name, "path": path, "if-no-files-found": "error", "retention-days": "1",
		})
}

func canonicalRunStep(step *yaml.Node, name, run string, env map[string]string) bool {
	keys := []string{"name", "shell", "run"}
	if env != nil {
		keys = append(keys, "env")
	}
	if !exactKeys(step, keys...) || scalar(step, "name") != name || scalar(step, "shell") != "bash" || scalar(step, "run") != run {
		return false
	}
	return env == nil || exactStringMap(value(step, "env"), env)
}

func stepsOf(job *yaml.Node) []*yaml.Node {
	steps := value(job, "steps")
	if steps == nil || steps.Kind != yaml.SequenceNode {
		return nil
	}
	return steps.Content
}

func contentsRead(node *yaml.Node) bool {
	return exactStringMap(value(node, "permissions"), map[string]string{"contents": "read"})
}

func value(node *yaml.Node, key string) *yaml.Node {
	value, _ := workflowpolicy.MappingValue(node, key)
	return value
}

func scalar(node *yaml.Node, key string) string {
	value := value(node, key)
	if value == nil || value.Kind != yaml.ScalarNode {
		return ""
	}
	return value.Value
}

func boolValue(node *yaml.Node, key string, want bool) bool {
	value := value(node, key)
	return value != nil && value.Kind == yaml.ScalarNode && value.Tag == "!!bool" && value.Value == fmt.Sprint(want)
}

func exactKeys(node *yaml.Node, keys ...string) bool {
	return workflowpolicy.HasExactMappingKeys(node, keys...)
}

func exactStringMap(node *yaml.Node, want map[string]string) bool {
	if node == nil || node.Kind != yaml.MappingNode || len(node.Content) != len(want)*2 {
		return false
	}
	keys := make([]string, 0, len(want))
	for key, expected := range want {
		if scalar(node, key) != expected {
			return false
		}
		keys = append(keys, key)
	}
	return exactKeys(node, keys...)
}
