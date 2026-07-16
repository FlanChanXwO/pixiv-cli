package main

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestRequireOnlyMappingKeysPreservesRequiredKeyError(t *testing.T) {
	t.Parallel()

	err := requireOnlyMappingKeys(mappingNode("unexpected", scalarNode("value")), "required")
	if err == nil || err.Error() != "must contain exactly the required keys" {
		t.Fatalf("requireOnlyMappingKeys() error = %v, want exact required-key error", err)
	}
}

func TestCheckWorkflowRejectsSoftFailedOrSkippedQualityGate(t *testing.T) {
	t.Parallel()

	gates := []struct {
		name    string
		command string
	}{
		{name: "Rust format", command: "cargo fmt --check"},
		{name: "Rust clippy", command: "cargo clippy --locked --offline --all-targets -- -D warnings"},
		{name: "pre-commit install", command: "python -m pip install --disable-pip-version-check pre-commit==4.6.0"},
		{name: "pre-commit", command: "python -m pre_commit run --all-files"},
	}
	for _, gate := range gates {
		for _, mutation := range []struct {
			name  string
			key   string
			value string
		}{
			{name: "soft failure", key: "continue-on-error", value: "true"},
			{name: "conditional skip", key: "if", value: "false"},
		} {
			t.Run(gate.name+" "+mutation.name, func(t *testing.T) {
				t.Parallel()
				root := releaseWorkflowRoot(t)
				step := stepWithRun(t, jobNode(t, root, "build"), gate.command)
				appendMappingValue(t, step, mutation.key, scalarNode(mutation.value))
				body, err := yaml.Marshal(root)
				if err != nil {
					t.Fatalf("marshal mutated workflow: %v", err)
				}
				err = checkWorkflow(body)
				if err == nil || !strings.Contains(err.Error(), "must not define continue-on-error or if") {
					t.Fatalf("policy error = %v, want unconditional quality-gate rejection", err)
				}
			})
		}
	}
}

func TestCheckWorkflowRejectsAmbiguousYAMLMutations(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name   string
		want   string
		mutate func(t *testing.T, root *yaml.Node)
	}{
		{
			name: "duplicate publish dependency",
			want: "duplicate mapping key \"needs\"",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				appendMappingValue(t, jobNode(t, root, "publish"), "needs", scalarNode("build"))
			},
		},
		{
			name: "root defaults change the working directory",
			want: "workflow root must not declare defaults",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				appendMappingValue(t, root, "defaults", mappingNode(
					"run", mappingNode("working-directory", scalarNode("/tmp")),
				))
			},
		},
		{
			name: "matrix entry has duplicate artifact",
			want: "duplicate mapping key \"artifact\"",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				include := requireMappingValue(t, requireMappingValue(t, requireMappingValue(t, jobNode(t, root, "build"), "strategy"), "matrix"), "include")
				appendMappingValue(t, include.Content[0], "artifact", scalarNode("rewritten-artifact"))
			},
		},
		{
			name: "YAML alias",
			want: "workflow must not use YAML aliases",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				checkout := findFirstUses(t, root)
				checkout.Anchor = "checkout"
				appendMappingValue(t, root, "alias", &yaml.Node{Kind: yaml.AliasNode, Value: "checkout", Alias: checkout})
			},
		},
		{
			name: "YAML merge key",
			want: "workflow must not use YAML merge keys",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				appendMappingValue(t, root, "<<", mappingNode("env", mappingNode("UNSAFE", scalarNode("true"))))
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			root := releaseWorkflowRoot(t)
			test.mutate(t, root)
			body, err := yaml.Marshal(root)
			if err != nil {
				t.Fatalf("marshal mutated workflow: %v", err)
			}
			err = checkWorkflow(body)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("policy error = %v, want ambiguous-YAML rejection %q", err, test.want)
			}
		})
	}
}

func TestCheckWorkflowRejectsRequiredJobExecutionOverrides(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name  string
		job   string
		key   string
		value *yaml.Node
		want  string
	}{
		{name: "validate if", job: "validate", key: "if", value: scalarNode("false"), want: "validate job must not define if or continue-on-error"},
		{name: "validate continue on error", job: "validate", key: "continue-on-error", value: scalarNode("true"), want: "validate job must not define if or continue-on-error"},
		{name: "build if", job: "build", key: "if", value: scalarNode("false"), want: "build job must not define if or continue-on-error"},
		{name: "build continue on error", job: "build", key: "continue-on-error", value: scalarNode("true"), want: "build job must not define if or continue-on-error"},
		{name: "verify if", job: "verify_release_source", key: "if", value: scalarNode("false"), want: "verify_release_source job must not define if or continue-on-error"},
		{name: "verify continue on error", job: "verify_release_source", key: "continue-on-error", value: scalarNode("true"), want: "verify_release_source job must not define if or continue-on-error"},
		{name: "publish if", job: "publish", key: "if", value: scalarNode("always()"), want: "publish job must not define if or continue-on-error"},
		{name: "publish continue on error", job: "publish", key: "continue-on-error", value: scalarNode("true"), want: "publish job must not define if or continue-on-error"},
		{name: "build defaults", job: "build", key: "defaults", value: mappingNode("run", mappingNode("working-directory", scalarNode("/tmp"))), want: "build job must not declare defaults"},
		{name: "verify defaults", job: "verify_release_source", key: "defaults", value: mappingNode("run", mappingNode("working-directory", scalarNode("/tmp"))), want: "verify_release_source job must not declare defaults"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			root := releaseWorkflowRoot(t)
			appendMappingValue(t, jobNode(t, root, test.job), test.key, test.value)
			body, err := yaml.Marshal(root)
			if err != nil {
				t.Fatalf("marshal mutated workflow: %v", err)
			}
			err = checkWorkflow(body)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("policy error = %v, want required-job execution rejection %q", err, test.want)
			}
		})
	}
}

func TestCheckWorkflowRejectsConditionalTrustGate(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name  string
		key   string
		value string
	}{
		{name: "if", key: "if", value: "false"},
		{name: "continue on error", key: "continue-on-error", value: "true"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			root := releaseWorkflowRoot(t)
			step := stepWithRun(t, jobNode(t, root, "verify_release_source"), "git merge-base --is-ancestor")
			appendMappingValue(t, step, test.key, scalarNode(test.value))
			body, err := yaml.Marshal(root)
			if err != nil {
				t.Fatalf("marshal mutated workflow: %v", err)
			}
			err = checkWorkflow(body)
			if err == nil || !strings.Contains(err.Error(), "default-branch ancestry gate must not define if or continue-on-error") {
				t.Fatalf("policy error = %v, want unconditional trust-gate rejection", err)
			}
		})
	}
}

func TestCheckWorkflowRejectsNonDirectQualityGateRuns(t *testing.T) {
	t.Parallel()

	for _, mutation := range []struct {
		name string
		run  func(string) string
	}{
		{name: "conditional shell", run: func(gate string) string { return "if false; then\n  " + gate + "\nfi" }},
		{name: "softened with or true", run: func(gate string) string { return gate + " || true" }},
	} {
		for _, gate := range requiredQualityGateCommands() {
			t.Run(gate+" "+mutation.name, func(t *testing.T) {
				t.Parallel()
				root := releaseWorkflowRoot(t)
				step := stepWithRun(t, jobNode(t, root, "build"), gate)
				requireMappingValue(t, step, "run").Value = mutation.run(gate)
				body, err := yaml.Marshal(root)
				if err != nil {
					t.Fatalf("marshal mutated workflow: %v", err)
				}
				err = checkWorkflow(body)
				if err == nil || !strings.Contains(err.Error(), gate) {
					t.Fatalf("policy error = %v, want direct quality-gate rejection for %q", err, gate)
				}
			})
		}
	}

	t.Run("unrelated control flow", func(t *testing.T) {
		t.Parallel()
		root := releaseWorkflowRoot(t)
		step := stepWithRun(t, jobNode(t, root, "build"), "go test ./...")
		requireMappingValue(t, step, "run").Value = "go test ./...\nif true; then\n  :\nfi"
		body, err := yaml.Marshal(root)
		if err != nil {
			t.Fatalf("marshal mutated workflow: %v", err)
		}
		err = checkWorkflow(body)
		if err == nil || !strings.Contains(err.Error(), "go test ./...") {
			t.Fatalf("policy error = %v, want direct quality-gate rejection", err)
		}
	})
}

func TestCheckWorkflowRejectsQualityGateExecutionOverrides(t *testing.T) {
	t.Parallel()

	for _, gate := range requiredQualityGateCommands() {
		for _, mutation := range []struct {
			name   string
			mutate func(t *testing.T, step *yaml.Node)
		}{
			{name: "environment", mutate: func(t *testing.T, step *yaml.Node) {
				appendMappingValue(t, step, "env", mappingNode("PWD", scalarNode("/tmp")))
			}},
			{name: "defaults", mutate: func(t *testing.T, step *yaml.Node) {
				appendMappingValue(t, step, "defaults", mappingNode("run", mappingNode("working-directory", scalarNode("/tmp"))))
			}},
			{name: "shell override", mutate: func(t *testing.T, step *yaml.Node) { requireMappingValue(t, step, "shell").Value = "bash -e {0}" }},
		} {
			t.Run(gate+" "+mutation.name, func(t *testing.T) {
				t.Parallel()
				root := releaseWorkflowRoot(t)
				step := stepWithRun(t, jobNode(t, root, "build"), gate)
				mutation.mutate(t, step)
				body, err := yaml.Marshal(root)
				if err != nil {
					t.Fatalf("marshal mutated workflow: %v", err)
				}
				err = checkWorkflow(body)
				if err == nil || !strings.Contains(err.Error(), gate) {
					t.Fatalf("policy error = %v, want quality execution-override rejection for %q", err, gate)
				}
			})
		}
	}
}
