package filename_test

import (
	"testing"

	. "github.com/FlanChanXwO/pixiv-cli/internal/media/filename"
)

func TestGenerateCheckedSupportsExtendedPlaceholdersAndExplicitNum(t *testing.T) {
	got, err := GenerateChecked(Fields{
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
	data := Fields{ID: 1, CreateDate: "not-a-date"}
	if _, err := GenerateChecked(data, 0, "{id}"); err != nil {
		t.Fatalf("GenerateChecked() unexpected error: %v", err)
	}
	if _, err := GenerateChecked(data, 0, "{date}_{id}"); err == nil {
		t.Fatal("GenerateChecked() error = nil")
	}
}

func TestBuildRelativeDirectoryRejectsUnsafeSegments(t *testing.T) {
	if _, err := BuildRelativeDirectory("{author}/{date}", Fields{Author: "artist", CreateDate: "2026-08-01T00:00:00Z"}, 0); err != nil {
		t.Fatal(err)
	}
	for _, template := range []string{"/absolute", "a//b", "../work", "a/./b"} {
		if _, err := BuildRelativeDirectory(template, Fields{}, 0); err == nil {
			t.Fatalf("BuildRelativeDirectory(%q) error = nil", template)
		}
	}
}

func TestDirectoryTemplateDoesNotAppendFilenamePageSuffixOrRejectEmptyTagsAtPreflight(t *testing.T) {
	if err := ValidateDirectoryTemplate("{author}/{tags}"); err != nil {
		t.Fatalf("ValidateDirectoryTemplate() error = %v", err)
	}
	directory, err := BuildRelativeDirectory("{author}/{num}", Fields{Author: "artist", PageCount: 3}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if directory != "artist/1" {
		t.Fatalf("BuildRelativeDirectory() = %q", directory)
	}
	if _, err := BuildRelativeDirectory("{tags}", Fields{}, 0); err == nil {
		t.Fatal("empty dynamic directory segment must fail for the concrete artwork")
	}
}
