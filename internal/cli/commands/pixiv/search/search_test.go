package search

import (
	"bytes"
	"context"
	"strings"
	"testing"

	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

func TestCommandRejectsInvalidFilterBeforeOpeningClient(t *testing.T) {
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
	cmd.SetArgs([]string{"miku", "--resolution", "impossible"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "resolution must be one of") {
		t.Fatalf("expected resolution validation error, got %v", err)
	}
	if opened {
		t.Fatal("opened SDK client before validating search filters")
	}
}
