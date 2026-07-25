package main

import (
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/scripts/internal/workflowpolicy"
	"gopkg.in/yaml.v3"
)

func TestReleaseWorkflowHasProtectedAuthenticatedE2EGate(t *testing.T) {
	t.Parallel()

	root := releaseWorkflowRoot(t)
	e2e := jobNode(t, root, "e2e")
	if got := requireMappingValue(t, e2e, "environment").Value; got != "pixiv-e2e" {
		t.Fatalf("e2e environment = %q, want pixiv-e2e", got)
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
	run := requireMappingValue(t, steps[2], "run").Value
	if !strings.Contains(run, "go test ./e2e -count=1 -v") {
		t.Fatal("e2e gate must run the complete e2e suite")
	}
	env := requireMappingValue(t, steps[2], "env")
	if got := requireMappingValue(t, env, "PIXIV_E2E_REFRESH_TOKEN").Value; got != "${{ secrets.PIXIV_E2E_REFRESH_TOKEN }}" {
		t.Fatal("e2e refresh token must come from the protected environment secret")
	}
	for _, name := range []string{
		"PIXIV_E2E_SFW_ILLUST_ID",
		"PIXIV_E2E_R18_ILLUST_ID",
		"PIXIV_E2E_R18_UGOIRA_ID",
		"PIXIV_E2E_ILLUST_SEARCH_WORD",
		"PIXIV_E2E_DISCOVERY_WORD",
	} {
		if got := requireMappingValue(t, env, name).Value; got != "${{ vars."+name+" }}" {
			t.Fatalf("e2e %s must come from the protected environment variable", name)
		}
	}
	if workflowpolicy.ContainsSecretReference(steps[0]) || workflowpolicy.ContainsSecretReference(steps[1]) {
		t.Fatal("e2e checkout and setup steps must not reference secrets")
	}

	buildNeeds := requireMappingValue(t, jobNode(t, root, "build"), "needs").Content
	if len(buildNeeds) != 2 || buildNeeds[0].Value != "validate" || buildNeeds[1].Value != "e2e" {
		t.Fatal("build must wait for both validate and e2e")
	}
}

func TestCheckWorkflowRejectsAuthenticatedE2EGateMutations(t *testing.T) {
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
			name: "e2e environment changes",
			want: "e2e environment must be pixiv-e2e",
			mutate: func(t *testing.T, root *yaml.Node) {
				requireMappingValue(t, jobNode(t, root, "e2e"), "environment").Value = "release"
			},
		},
		{
			name: "checkout receives e2e secret",
			want: "e2e job must not reference secrets before its authenticated test step",
			mutate: func(t *testing.T, root *yaml.Node) {
				appendMappingValue(t, checkoutStep(t, jobNode(t, root, "e2e")), "env", mappingNode("LEAK", scalarNode("${{ secrets.PIXIV_E2E_REFRESH_TOKEN }}")))
			},
		},
		{
			name: "refresh token secret changes",
			want: "PIXIV_E2E_REFRESH_TOKEN",
			mutate: func(t *testing.T, root *yaml.Node) {
				env := requireMappingValue(t, requireMappingValue(t, jobNode(t, root, "e2e"), "steps").Content[2], "env")
				requireMappingValue(t, env, "PIXIV_E2E_REFRESH_TOKEN").Value = "${{ secrets.OTHER }}"
			},
		},
		{
			name: "required input changes source",
			want: "PIXIV_E2E_DISCOVERY_WORD",
			mutate: func(t *testing.T, root *yaml.Node) {
				env := requireMappingValue(t, requireMappingValue(t, jobNode(t, root, "e2e"), "steps").Content[2], "env")
				requireMappingValue(t, env, "PIXIV_E2E_DISCOVERY_WORD").Value = "hard-coded"
			},
		},
		{
			name: "direct test command is softened",
			want: "authenticated e2e step must run the complete direct E2E command sequence",
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
