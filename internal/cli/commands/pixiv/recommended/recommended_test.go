package recommended

import (
	"bytes"
	"context"
	"strings"
	"testing"

	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

func TestCommandRejectsKindTypeConflictBeforeOpeningClient(t *testing.T) {
	opened := false
	cmd := New(Dependencies{
		Input:      strings.NewReader(""),
		Output:     &bytes.Buffer{},
		UsageError: func(err error) error { return err },
		JSONOut:    func(*bool) (bool, error) { return false, nil },
		Pooled: func(context.Context, Request, func(context.Context, *pixiv.Client) (bool, error)) error {
			opened = true
			return nil
		},
	})
	cmd.SetArgs([]string{"illust", "--type", "novel"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "KIND cannot be combined") {
		t.Fatalf("expected KIND/--type validation error, got %v", err)
	}
	if opened {
		t.Fatal("opened SDK client before validating recommendation kind")
	}
}

func TestCommandRejectsInvalidContentTypeForPositionalKind(t *testing.T) {
	opened := false
	cmd := New(Dependencies{
		Input:      strings.NewReader(""),
		Output:     &bytes.Buffer{},
		UsageError: func(err error) error { return err },
		JSONOut:    func(*bool) (bool, error) { return false, nil },
		Pooled: func(context.Context, Request, func(context.Context, *pixiv.Client) (bool, error)) error {
			opened = true
			return nil
		},
	})
	cmd.SetArgs([]string{"artwork", "--content-type", "bogus"})

	err := cmd.Execute()

	if err == nil || !strings.Contains(err.Error(), "content-type must be one of all, illust, manga") {
		t.Fatalf("expected positional content-type validation error, got %v", err)
	}
	if opened {
		t.Fatal("opened SDK client before validating recommendation content type")
	}
}
