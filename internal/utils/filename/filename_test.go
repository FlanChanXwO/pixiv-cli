package filename

import "testing"

func TestGenerateSanitizedFilename(t *testing.T) {
	t.Parallel()

	got := Generate(FilenameData{ID: 123, Author: `x*y`, Title: `a/b:c`, PageCount: 2}, 1, "{author}_{id}_{title}")
	if got != "x_y_123_a_b_c_p1" {
		t.Fatalf("Generate() = %q", got)
	}
}
