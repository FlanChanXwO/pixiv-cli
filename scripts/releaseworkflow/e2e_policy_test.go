package main

import (
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/scripts/internal/workflowpolicy"
	"gopkg.in/yaml.v3"
)

func TestReleaseWorkflowHasOfflineSDKContractGate(t *testing.T) {
	t.Parallel()

	root := releaseWorkflowRoot(t)
	e2e := jobNode(t, root, "e2e")
	if got := requireMappingValue(t, e2e, "name").Value; got != "SDK E2E contract gate" {
		t.Fatalf("e2e job name = %q", got)
	}
	if _, ok := workflowpolicy.MappingValue(e2e, "environment"); ok {
		t.Fatal("offline e2e contract gate must not require a credential environment")
	}
	if workflowpolicy.ContainsSecretReference(e2e) {
		t.Fatal("offline e2e contract gate must not reference secrets")
	}
	if got := requireMappingValue(t, e2e, "runs-on").Value; got != "ubuntu-24.04" {
		t.Fatalf("e2e runner = %q, want ubuntu-24.04", got)
	}
	if got := requireMappingValue(t, requireMappingValue(t, e2e, "permissions"), "contents").Value; got != "read" {
		t.Fatalf("e2e contents permission = %q, want read", got)
	}
	if got := requireMappingValue(t, e2e, "needs").Content; len(got) != 1 || got[0].Value != "validate" {
		t.Fatal("e2e job must run only after release-tag validation")
	}

	steps := requireMappingValue(t, e2e, "steps").Content
	if len(steps) != 3 {
		t.Fatalf("e2e step count = %d, want 3", len(steps))
	}
	if got := requireMappingValue(t, steps[2], "name").Value; got != "Run offline SDK E2E contract tests" {
		t.Fatalf("e2e contract step name = %q", got)
	}
	if got := requireMappingValue(t, steps[2], "run").Value; !equalCommands(splitCommands(got), splitCommands(offlineE2ECommands)) {
		t.Fatalf("e2e contract command = %q", got)
	}
	if workflowpolicy.ContainsSecretReference(steps[0]) || workflowpolicy.ContainsSecretReference(steps[1]) {
		t.Fatal("e2e checkout and setup steps must not reference secrets")
	}

	buildNeeds := requireMappingValue(t, jobNode(t, root, "build"), "needs").Content
	if len(buildNeeds) != 2 || buildNeeds[0].Value != "validate" || buildNeeds[1].Value != "e2e" {
		t.Fatal("build must wait for validation and offline SDK contract gate")
	}
}

func TestCheckWorkflowRejectsOfflineE2EContractMutations(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name   string
		want   string
		mutate func(t *testing.T, root *yaml.Node)
	}{
		{
			name: "e2e is conditionally skipped",
			want: "e2e job must not define if or continue-on-error",
			mutate: func(t *testing.T, root *yaml.Node) {
				appendMappingValue(t, jobNode(t, root, "e2e"), "if", scalarNode("false"))
			},
		},
		{
			name: "e2e obtains credentials",
			want: "e2e job: must contain exactly the required keys",
			mutate: func(t *testing.T, root *yaml.Node) {
				appendMappingValue(t, jobNode(t, root, "e2e"), "env", mappingNode("TOKEN", scalarNode("${{ secrets.TOKEN }}")))
			},
		},
		{
			name: "direct test command is softened",
			want: "offline SDK contract step must run the exact no-credential E2E command",
			mutate: func(t *testing.T, root *yaml.Node) {
				step := requireMappingValue(t, jobNode(t, root, "e2e"), "steps").Content[2]
				requireMappingValue(t, step, "run").Value += " || true"
			},
		},
		{
			name: "build does not wait for e2e",
			want: "needs must equal",
			mutate: func(t *testing.T, root *yaml.Node) {
				requireMappingValue(t, jobNode(t, root, "build"), "needs").Content = []*yaml.Node{scalarNode("validate")}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			root := releaseWorkflowRoot(t)
			test.mutate(t, root)
			err := checkWorkflow(mustMarshalYAML(t, root))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("policy error = %v, want rejection containing %q", err, test.want)
			}
		})
	}
}

func TestRecoveryOverlayIncludesE2EPolicyAndMutationTests(t *testing.T) {
	t.Parallel()

	root := releaseWorkflowRoot(t)
	step := stepWithRun(t, jobNode(t, root, "build"), `git archive --format=tar "$GITHUB_SHA"`)
	run := requireMappingValue(t, step, "run").Value
	if strings.Count(run, "scripts/releaseworkflow/e2e_policy.go") != 1 {
		t.Fatal("recovery overlay must include the E2E policy implementation in its audited path list")
	}
	if strings.Count(run, "scripts/releaseworkflow/e2e_policy_test.go") != 1 {
		t.Fatal("recovery overlay must include the E2E policy mutation tests in its audited path list")
	}
}
