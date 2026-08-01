package filename

import "testing"

func TestGenerateSanitizedFilename(t *testing.T) {
	t.Parallel()

	got := Generate(FilenameData{ID: 123, Author: `x*y`, Title: `a/b:c`, PageCount: 2}, 1, "{author}_{id}_{title}")
	if got != "x_y_123_a_b_c_p1" {
		t.Fatalf("Generate() = %q", got)
	}
}

func TestGenerateCheckedSupportsExtendedPlaceholdersAndExplicitNum(t *testing.T) {
	got, err := GenerateChecked(FilenameData{
		ID: 123, Author: "artist", AuthorID: 9, Title: "work", PageCount: 3,
		CreateDate: "2026-08-01T13:14:15+09:00", Tags: []string{"a", "b"},
	}, 1, "{author_id}_{date}_{tags}_{num}")
	if err != nil {
		t.Fatal(err)
	}
	if got != "9_2026-08-01_a,b_1" {
		t.Fatalf("GenerateChecked() = %q", got)
	}
}

func TestGenerateCheckedRejectsInvalidDateOnlyWhenReferenced(t *testing.T) {
	data := FilenameData{ID: 1, CreateDate: "not-a-date"}
	if _, err := GenerateChecked(data, 0, "{id}"); err != nil {
		t.Fatalf("GenerateChecked() unexpected error: %v", err)
	}
	if _, err := GenerateChecked(data, 0, "{date}_{id}"); err == nil {
		t.Fatal("GenerateChecked() error = nil")
	}
}

func TestBuildRelativeDirectoryRejectsUnsafeSegments(t *testing.T) {
	if _, err := BuildRelativeDirectory("{author}/{date}", FilenameData{Author: "artist", CreateDate: "2026-08-01T00:00:00Z"}, 0); err != nil {
		t.Fatal(err)
	}
	for _, template := range []string{"/absolute", "a//b", "../work", "a/./b"} {
		if _, err := BuildRelativeDirectory(template, FilenameData{}, 0); err == nil {
			t.Fatalf("BuildRelativeDirectory(%q) error = nil", template)
		}
	}
}

func TestDirectoryTemplateDoesNotAppendFilenamePageSuffixOrRejectEmptyTagsAtPreflight(t *testing.T) {
	if err := ValidateDirectoryTemplate("{author}/{tags}"); err != nil {
		t.Fatalf("ValidateDirectoryTemplate() error = %v", err)
	}
	directory, err := BuildRelativeDirectory("{author}/{num}", FilenameData{Author: "artist", PageCount: 3}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if directory != "artist/1" {
		t.Fatalf("BuildRelativeDirectory() = %q", directory)
	}
	if _, err := BuildRelativeDirectory("{tags}", FilenameData{}, 0); err == nil {
		t.Fatal("empty dynamic directory segment must fail for the concrete artwork")
	}
}
