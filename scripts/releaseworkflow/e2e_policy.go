package main

import (
	"errors"
	"fmt"
	"regexp"

	"github.com/FlanChanXwO/pixiv-cli/scripts/internal/workflowpolicy"
	"gopkg.in/yaml.v3"
)

var authenticatedE2EVariableNames = []string{
	"PIXIV_E2E_SFW_ILLUST_ID",
	"PIXIV_E2E_R18_ILLUST_ID",
	"PIXIV_E2E_R18_UGOIRA_ID",
	"PIXIV_E2E_ILLUST_SEARCH_WORD",
	"PIXIV_E2E_DISCOVERY_WORD",
	"PIXIV_E2E_PROXY",
}

const e2eRecoveryOverlayCommands = `
set -euo pipefail
test -z "$(git status --porcelain=v1 --untracked-files=all)"
test -z "$(git diff --cached --name-only)"
e2e_recovery_overlay_paths=(
  e2e/authenticated_r18_regression_test.go
  e2e/pixiv_binary_test.go
)
git archive --format=tar "$GITHUB_SHA" -- "${e2e_recovery_overlay_paths[@]}" | tar -xf -
actual_overlay_paths="$(
  {
    git diff --name-only
    git ls-files --others --exclude-standard
  } | LC_ALL=C sort -u
)"
test -n "$actual_overlay_paths"
while IFS= read -r path; do
  overlay_path_allowed=false
  for allowed_path in "${e2e_recovery_overlay_paths[@]}"; do
    if [ "$path" = "$allowed_path" ]; then
      overlay_path_allowed=true
      break
    fi
  done
  if [ "$overlay_path_allowed" != true ]; then
    printf 'unexpected E2E recovery overlay path: %s\n' "$path" >&2
    exit 1
  fi
done <<< "$actual_overlay_paths"
test -z "$(git diff --cached --name-only)"
`

func checkE2EJob(job *yaml.Node) error {
	if err := requireRequiredJobExecution(job, "e2e job"); err != nil {
		return err
	}
	if err := requireOnlyMappingKeys(job, "name", "needs", "runs-on", "environment", "permissions", "steps"); err != nil {
		return fmt.Errorf("e2e job: %w", err)
	}
	if err := workflowpolicy.RequireScalar(job, "name", "Authenticated Pixiv E2E"); err != nil {
		return fmt.Errorf("e2e job: %w", err)
	}
	if err := requireExactStringSequence(job, "needs", "validate"); err != nil {
		return fmt.Errorf("e2e job: %w", err)
	}
	if err := workflowpolicy.RequireScalar(job, "runs-on", "ubuntu-24.04"); err != nil {
		return fmt.Errorf("e2e job: %w", err)
	}
	if err := workflowpolicy.RequireScalar(job, "environment", "pixiv-e2e"); err != nil {
		return errors.New("e2e environment must be pixiv-e2e")
	}
	if err := requireContentsPermission(job, "read"); err != nil {
		return fmt.Errorf("e2e job: %w", err)
	}
	steps, err := jobSteps(job)
	if err != nil || len(steps) != 4 {
		return errors.New("e2e job must contain only checkout, Go setup, the audited recovery overlay, and the authenticated test gate")
	}
	if err := requireCanonicalCheckout(steps[0], "e2e job", checkoutWithRequirement{"fetch-depth", "0"}, checkoutWithRequirement{"persist-credentials", "false"}, checkoutWithRequirement{"ref", "${{ env.RELEASE_TAG }}"}); err != nil {
		return err
	}
	if err := requireExactActionStep(steps[1], "e2e Go setup", setupGoAction, map[string]string{"go-version": "1.26.3"}); err != nil {
		return err
	}
	if err := requireE2ERecoveryOverlayStep(steps[2]); err != nil {
		return err
	}
	return checkAuthenticatedE2EStep(steps[3])
}

// requireE2ERecoveryOverlayStep 限定恢复只能修补两个已验证失败的 E2E 契约测试；
// 不可变 tag 的生产源码、发布资产及受保护凭据均不参与该覆盖。
func requireE2ERecoveryOverlayStep(step *yaml.Node) error {
	if err := requireCanonicalConditionalRunStep(step, "E2E recovery overlay", "github.event_name == 'workflow_dispatch'", e2eRecoveryOverlayCommands); err != nil {
		return errors.New("E2E recovery overlay must use only the exact audited test paths and verifier")
	}
	if err := workflowpolicy.RequireScalar(step, "name", "Apply the audited E2E test recovery overlay"); err != nil {
		return errors.New("E2E recovery overlay must keep its canonical name")
	}
	return nil
}

func checkAuthenticatedE2EStep(step *yaml.Node) error {
	const commands = `
set -euo pipefail
test -n "$PIXIV_E2E_REFRESH_TOKEN"
test -n "$PIXIV_E2E_SFW_ILLUST_ID"
test -n "$PIXIV_E2E_R18_ILLUST_ID"
test -n "$PIXIV_E2E_R18_UGOIRA_ID"
test -n "$PIXIV_E2E_ILLUST_SEARCH_WORD"
test -n "$PIXIV_E2E_DISCOVERY_WORD"
PIXIV_E2E_REAL_API=1 PIXIV_E2E_WEB_API=1 PIXIV_WEB_API_PROXY="$PIXIV_E2E_PROXY" go test ./e2e -count=1 -v`

	if err := requireOnlyMappingKeys(step, "name", "shell", "env", "run"); err != nil {
		return errors.New("authenticated e2e step must use only its explicit environment and direct bash command")
	}
	if err := workflowpolicy.RequireScalar(step, "name", "Run authenticated Pixiv E2E"); err != nil {
		return errors.New("authenticated e2e step must keep its canonical name")
	}
	if err := workflowpolicy.RequireScalar(step, "shell", "bash"); err != nil || !equalCommands(splitCommands(requireRunValue(step)), splitCommands(commands)) {
		return errors.New("authenticated e2e step must run the complete direct E2E command sequence")
	}
	env, ok := workflowpolicy.MappingValue(step, "env")
	if !ok || env.Kind != yaml.MappingNode {
		return errors.New("authenticated e2e step must declare its protected environment")
	}
	keys := append([]string{"PIXIV_E2E_REFRESH_TOKEN"}, authenticatedE2EVariableNames...)
	if err := requireOnlyMappingKeys(env, keys...); err != nil {
		return errors.New("authenticated e2e step must declare only the required secret, inputs, and optional proxies")
	}
	if err := requireExpectedE2ESecret(env, "PIXIV_E2E_REFRESH_TOKEN"); err != nil {
		return err
	}
	for _, name := range authenticatedE2EVariableNames {
		value, ok := workflowpolicy.MappingValue(env, name)
		if !ok || value.Kind != yaml.ScalarNode || !expectedE2EVariableExpression(name).MatchString(value.Value) {
			return fmt.Errorf("authenticated e2e step must map %s from its protected environment variable", name)
		}
	}
	return nil
}

func requireExactStringSequence(mapping *yaml.Node, key string, values ...string) error {
	sequence, ok := workflowpolicy.MappingValue(mapping, key)
	if !ok || sequence.Kind != yaml.SequenceNode || len(sequence.Content) != len(values) {
		return fmt.Errorf("%s must equal %q", key, values)
	}
	for index, want := range values {
		if sequence.Content[index].Kind != yaml.ScalarNode || sequence.Content[index].Value != want {
			return fmt.Errorf("%s must equal %q", key, values)
		}
	}
	return nil
}

func expectedE2EVariableExpression(name string) *regexp.Regexp {
	quotedName := regexp.QuoteMeta(name)
	return regexp.MustCompile(`^\$\{\{\s*vars\s*(?:\.\s*` + quotedName + `|\[\s*['"]` + quotedName + `['"]\s*\])\s*\}\}$`)
}

func requireExpectedE2ESecret(env *yaml.Node, key string) error {
	value, ok := workflowpolicy.MappingValue(env, key)
	if !ok || value.Kind != yaml.ScalarNode || !expectedSigningSecretExpression(key).MatchString(value.Value) {
		return fmt.Errorf("authenticated e2e step must map %s from the protected environment secret", key)
	}
	return nil
}

func checkE2ESecretReachability(job *yaml.Node) error {
	for index := 0; index+1 < len(job.Content); index += 2 {
		if job.Content[index].Value != "steps" && workflowpolicy.ContainsSecretReference(job.Content[index+1]) {
			return errors.New("e2e job must not reference secrets outside its authenticated test step")
		}
	}
	steps, err := jobSteps(job)
	if err != nil {
		return err
	}
	for index, step := range steps {
		if index != len(steps)-1 && workflowpolicy.ContainsSecretReference(step) {
			return errors.New("e2e job must not reference secrets before its authenticated test step")
		}
		if index == len(steps)-1 && containsSigningSecretReferenceOutsideEnvironment(step) {
			return errors.New("authenticated e2e step must reference its secret only through its expected environment")
		}
	}
	if len(steps) == 0 {
		return nil
	}
	env, ok := workflowpolicy.MappingValue(steps[len(steps)-1], "env")
	if !ok || env.Kind != yaml.MappingNode {
		return errors.New("authenticated e2e step must declare its protected environment")
	}
	return requireExpectedE2ESecret(env, "PIXIV_E2E_REFRESH_TOKEN")
}
