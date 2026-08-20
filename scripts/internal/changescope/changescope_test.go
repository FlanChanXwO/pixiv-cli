package changescope

import "testing"

func TestDocsOnlyPaths(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name  string
		paths []string
		want  bool
	}{
		{
			name:  "approved documentation paths",
			paths: []string{"README.md", "README.zh-CN.md", "docs/zh-CN/maintainers/development.md", "changelog/unreleased/en.md", "skills/pixiv-cli/SKILL.md"},
			want:  true,
		},
		{name: "empty diff stays full", paths: nil, want: false},
		{name: "source change stays full", paths: []string{"README.md", "internal/cli/root.go"}, want: false},
		{name: "workflow change stays full", paths: []string{".github/workflows/ci.yml"}, want: false},
		{name: "dependency change stays full", paths: []string{"go.mod"}, want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := docsOnlyPaths(test.paths); got != test.want {
				t.Fatalf("docsOnlyPaths(%q) = %t, want %t", test.paths, got, test.want)
			}
		})
	}
}

func TestSplitNULPathsPreservesWhitespace(t *testing.T) {
	t.Parallel()

	got := splitNULPaths([]byte("README.md\x00docs/with space.md\x00"))
	want := []string{"README.md", "docs/with space.md"}
	if len(got) != len(want) {
		t.Fatalf("splitNULPaths() = %q, want %q", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("splitNULPaths()[%d] = %q, want %q", index, got[index], want[index])
		}
	}
}
