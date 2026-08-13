package releaseworkflow

import (
	"errors"
	"fmt"

	workflowyaml "github.com/FlanChanXwO/pixiv-cli/scripts/internal/workflow/yaml"
	"gopkg.in/yaml.v3"
)

// checkReleaseNotesAuditJob 将来源审计固定在受保护 publish job 之前。该 job
// 只有读取提交、PR 元数据和 tag 内说明所需的权限，不能读取发布签名 secret。
func checkReleaseNotesAuditJob(job *yaml.Node) error {
	const auditCommands = `
set -euo pipefail
previous_tag=$(git tag --merged "$RELEASE_TAG^" --sort=-version:refname | head -n 1)
test -n "$previous_tag"
audit_report="$RUNNER_TEMP/release-notes-audit.json"
go run ./scripts/releasenotes audit \
--repo "$GITHUB_REPOSITORY" \
--from "$previous_tag" \
--to "$RELEASE_TAG" \
--output "$audit_report"
go run ./scripts/releasenotes validate \
--version "${RELEASE_TAG#v}" \
--dir "changelog/$RELEASE_TAG" \
--previous "$previous_tag" \
--audit "$audit_report"`

	if err := requireRequiredJobExecution(job, "release_notes_audit job"); err != nil {
		return err
	}
	if err := requireNoEnvironment(job, "release_notes_audit job"); err != nil {
		return err
	}
	if err := requireOnlyMappingKeys(job, "name", "needs", "runs-on", "permissions", "steps"); err != nil {
		return fmt.Errorf("release_notes_audit job: %w", err)
	}
	if err := workflowyaml.RequireScalar(job, "name", "Audit release notes provenance"); err != nil {
		return fmt.Errorf("release_notes_audit job: %w", err)
	}
	if err := workflowyaml.RequireScalar(job, "needs", "build_production"); err != nil {
		return fmt.Errorf("release_notes_audit job: %w", err)
	}
	if err := workflowyaml.RequireScalar(job, "runs-on", "ubuntu-24.04"); err != nil {
		return fmt.Errorf("release_notes_audit job: %w", err)
	}
	permissions, ok := workflowyaml.MappingValue(job, "permissions")
	if !ok || !workflowyaml.HasExactMappingKeys(permissions, "contents", "pull-requests") ||
		workflowyaml.RequireScalar(permissions, "contents", "read") != nil ||
		workflowyaml.RequireScalar(permissions, "pull-requests", "read") != nil {
		return errors.New("release_notes_audit job permissions must contain only contents: read and pull-requests: read")
	}
	steps, err := jobSteps(job)
	if err != nil || len(steps) != 3 {
		return errors.New("release_notes_audit job must contain only checkout, Go setup, and the source audit")
	}
	if err := requireCanonicalCheckout(steps[0], "release_notes_audit job", checkoutWithRequirement{"fetch-depth", "0"}, checkoutWithRequirement{"persist-credentials", "false"}, checkoutWithRequirement{"ref", "${{ env.RELEASE_TAG }}"}); err != nil {
		return err
	}
	if err := requireExactActionStep(steps[1], "release_notes_audit Go setup", setupGoAction, map[string]string{"go-version": "1.26.3"}); err != nil {
		return err
	}
	if err := requireOnlyMappingKeys(steps[2], "name", "shell", "env", "run"); err != nil ||
		workflowyaml.RequireScalar(steps[2], "name", "Audit release-note sources and validate the bilingual tag notes") != nil ||
		workflowyaml.RequireScalar(steps[2], "shell", "bash") != nil ||
		!equalCommands(splitCommands(requireRunValue(steps[2])), splitCommands(auditCommands)) {
		return errors.New("release_notes_audit job must run the canonical direct audit commands")
	}
	env, ok := workflowyaml.MappingValue(steps[2], "env")
	if !ok || !workflowyaml.HasExactMappingKeys(env, "GH_TOKEN") || workflowyaml.RequireScalar(env, "GH_TOKEN", "${{ github.token }}") != nil {
		return errors.New("release_notes_audit job must use only the ephemeral GitHub token")
	}
	return nil
}
