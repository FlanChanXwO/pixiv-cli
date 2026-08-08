package main

import (
	"errors"
	"fmt"

	"github.com/FlanChanXwO/pixiv-cli/scripts/internal/workflowpolicy"
	"gopkg.in/yaml.v3"
)

const offlineE2ECommands = `
set -euo pipefail
test -z "${PIXIV_SDK_E2E:-}"
test -z "${FANBOX_SDK_E2E:-}"
go test ./e2e -count=1 -v`

func checkE2EJob(job *yaml.Node) error {
	if err := requireRequiredJobExecution(job, "e2e job"); err != nil {
		return err
	}
	if err := requireOnlyMappingKeys(job, "name", "needs", "runs-on", "permissions", "steps"); err != nil {
		return fmt.Errorf("e2e job: %w", err)
	}
	if err := workflowpolicy.RequireScalar(job, "name", "SDK E2E contract gate"); err != nil {
		return fmt.Errorf("e2e job: %w", err)
	}
	if err := requireExactStringSequence(job, "needs", "validate"); err != nil {
		return fmt.Errorf("e2e job: %w", err)
	}
	if err := workflowpolicy.RequireScalar(job, "runs-on", "ubuntu-24.04"); err != nil {
		return fmt.Errorf("e2e job: %w", err)
	}
	if err := requireContentsPermission(job, "read"); err != nil {
		return fmt.Errorf("e2e job: %w", err)
	}
	if workflowpolicy.ContainsSecretReference(job) {
		return errors.New("e2e contract job must not reference credentials")
	}
	steps, err := jobSteps(job)
	if err != nil || len(steps) != 3 {
		return errors.New("e2e job must contain only checkout, Go setup, and the offline SDK contract gate")
	}
	if err := requireCanonicalCheckout(steps[0], "e2e job", checkoutWithRequirement{"fetch-depth", "0"}, checkoutWithRequirement{"persist-credentials", "false"}, checkoutWithRequirement{"ref", "${{ env.RELEASE_TAG }}"}); err != nil {
		return err
	}
	if err := requireExactActionStep(steps[1], "e2e Go setup", setupGoAction, map[string]string{"go-version": "1.26.3"}); err != nil {
		return err
	}
	if err := requireOnlyMappingKeys(steps[2], "name", "shell", "run"); err != nil {
		return errors.New("offline SDK contract step must contain only name, shell, and run")
	}
	if err := workflowpolicy.RequireScalar(steps[2], "name", "Run offline SDK E2E contract tests"); err != nil {
		return errors.New("offline SDK contract step must keep its canonical name")
	}
	if err := workflowpolicy.RequireScalar(steps[2], "shell", "bash"); err != nil || !equalCommands(splitCommands(requireRunValue(steps[2])), splitCommands(offlineE2ECommands)) {
		return errors.New("offline SDK contract step must run the exact no-credential E2E command")
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
