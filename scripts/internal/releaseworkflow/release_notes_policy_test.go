package releaseworkflow

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestCheckWorkflowRejectsReleaseNotesAuditPrivilegeOrDependencyChanges(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name   string
		want   string
		mutate func(t *testing.T, root *yaml.Node)
	}{
		{
			name: "permission elevated",
			want: "permissions must contain only contents: read and pull-requests: read",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				requireMappingValue(t, requireMappingValue(t, jobNode(t, root, "release_notes_audit"), "permissions"), "contents").Value = "write"
			},
		},
		{
			name: "secret reference",
			want: "non-release job must not reference secrets",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				step := requireMappingValue(t, jobNode(t, root, "release_notes_audit"), "steps").Content[2]
				appendMappingValue(t, step, "extra", scalarNode("${{ secrets.RELEASE_SIGNING_PRIVATE_KEY }}"))
			},
		},
		{
			name: "verify no longer needs audit",
			want: "needs must equal",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				needs := requireMappingValue(t, jobNode(t, root, "verify_release_source"), "needs")
				needs.Content = needs.Content[:1]
			},
		},
		{
			name: "unsupported audit flag",
			want: "canonical direct audit commands",
			mutate: func(t *testing.T, root *yaml.Node) {
				t.Helper()
				step := requireMappingValue(t, jobNode(t, root, "release_notes_audit"), "steps").Content[2]
				run := requireMappingValue(t, step, "run")
				run.Value = strings.Replace(
					run.Value,
					"go run ./scripts/cmd/releasenotes audit \\\n",
					"go run ./scripts/cmd/releasenotes audit --unexpected-option \\\n",
					1,
				)
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			root := releaseWorkflowRoot(t)
			test.mutate(t, root)
			err := checkWorkflow(mustMarshalYAML(t, root))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("policy error = %v, want %q", err, test.want)
			}
		})
	}
}
