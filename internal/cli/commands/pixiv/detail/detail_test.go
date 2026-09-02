package detail

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/spf13/cobra"
)

func TestCommandRejectsUnsupportedURLBeforeOpeningClient(t *testing.T) {
	opened := false
	data := Dependencies{
		Input:        strings.NewReader(""),
		Output:       &bytes.Buffer{},
		UsageError:   func(err error) error { return err },
		BuildRequest: func(*cobra.Command, Options) (Request, error) { return Request{}, nil },
		Pooled: func(context.Context, Request, func(context.Context, *pixiv.Client) (bool, error)) error {
			opened = true
			return nil
		},
		JSONOut: func(override *bool) (bool, error) { return override != nil && *override, nil },
	}

	cmd := New(data)
	cmd.SetArgs([]string{"https://www.pixiv.net/users/7?secret=must-not-echo"})
	err := cmd.Execute()

	if err == nil || !strings.Contains(err.Error(), "supported Pixiv") {
		t.Fatalf("expected unsupported URL error, got %v", err)
	}
	if opened {
		t.Fatal("client was opened before entity input validation")
	}
	if strings.Contains(err.Error(), "must-not-echo") {
		t.Fatal("unsupported URL query leaked into the error")
	}
}

func TestCommandRejectsContentForNonNovelBeforeOpeningClient(t *testing.T) {
	opened := false
	data := Dependencies{
		Input:        strings.NewReader(""),
		Output:       &bytes.Buffer{},
		UsageError:   func(err error) error { return err },
		BuildRequest: func(*cobra.Command, Options) (Request, error) { return Request{}, nil },
		Pooled: func(context.Context, Request, func(context.Context, *pixiv.Client) (bool, error)) error {
			opened = true
			return nil
		},
		JSONOut: func(override *bool) (bool, error) { return override != nil && *override, nil },
	}

	cmd := New(data)
	cmd.SetArgs([]string{"42", "--content"})
	err := cmd.Execute()

	if err == nil || !strings.Contains(err.Error(), "--content is only supported") {
		t.Fatalf("expected content validation error, got %v", err)
	}
	if opened {
		t.Fatal("client was opened before option validation")
	}
}

func TestCommandConsumesCanonicalArtworkRecordAsNDJSON(t *testing.T) {
	var output bytes.Buffer
	data := Dependencies{
		Input: strings.NewReader(`{"id":"42","type":"illust","url":"https://www.pixiv.net/artworks/42"}
`),
		Output:       &output,
		ErrorOutput:  &bytes.Buffer{},
		UsageError:   func(err error) error { return err },
		BuildRequest: func(*cobra.Command, Options) (Request, error) { return Request{}, nil },
		JSONOut:      func(override *bool) (bool, error) { return override != nil && *override, nil },
		OutputIsTTY:  func() bool { return false },
		Pooled: func(ctx context.Context, req Request, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
			_, err := attempt(ctx, nil)
			return err
		},
		FetchArtwork: func(context.Context, *pixiv.Client, int64) (pixiv.Artwork, error) {
			return pixiv.Artwork{ID: 42, Kind: pixiv.ArtworkKindIllustration, Title: "标题"}, nil
		},
	}

	cmd := New(data)
	cmd.SetArgs([]string{"--ndjson"})
	requireNoError(t, cmd.Execute())
	if !strings.Contains(output.String(), `"type":"illust"`) {
		t.Fatalf("expected canonical illust output, got %s", output.String())
	}
}

func requireNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func TestCommandConsumesRecordsAsJSONArray(t *testing.T) {
	var output bytes.Buffer
	data := testRecordDependencies(&output, strings.NewReader(`{"id":"42","type":"illust","url":"https://www.pixiv.net/artworks/42"}
`))
	cmd := New(data)
	cmd.SetArgs([]string{"--json"})
	requireNoError(t, cmd.Execute())
	var values []map[string]any
	if err := json.Unmarshal(output.Bytes(), &values); err != nil {
		t.Fatalf("expected JSON array, got %q: %v", output.String(), err)
	}
	if len(values) != 1 || values[0]["type"] != "illust" {
		t.Fatalf("unexpected array output: %s", output.String())
	}
}

func TestCommandRejectsJSONAndNDJSONTogether(t *testing.T) {
	data := testRecordDependencies(&bytes.Buffer{}, strings.NewReader(`{"id":"42","type":"illust","url":"https://www.pixiv.net/artworks/42"}
`))
	cmd := New(data)
	cmd.SetArgs([]string{"--json", "--ndjson"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "cannot be used together") {
		t.Fatalf("expected output mode conflict, got %v", err)
	}
}

func TestCommandRejectsRecordTypeMismatch(t *testing.T) {
	var diagnostics bytes.Buffer
	data := testRecordDependencies(&bytes.Buffer{}, strings.NewReader(`{"id":"42","type":"illust","url":"https://www.pixiv.net/artworks/42"}
`))
	data.ErrorOutput = &diagnostics
	cmd := New(data)
	cmd.SetArgs([]string{"--ndjson", "--type", "novel"})
	if err := cmd.Execute(); err == nil || !strings.Contains(diagnostics.String(), "incompatible") {
		t.Fatalf("expected type mismatch, got err=%v diagnostics=%s", err, diagnostics.String())
	}
}

func testRecordDependencies(output io.Writer, input io.Reader) Dependencies {
	return Dependencies{
		Input: input, Output: output, ErrorOutput: &bytes.Buffer{},
		UsageError:   func(err error) error { return err },
		BuildRequest: func(*cobra.Command, Options) (Request, error) { return Request{}, nil },
		JSONOut:      func(override *bool) (bool, error) { return override != nil && *override, nil },
		Pooled: func(ctx context.Context, req Request, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
			_, err := attempt(ctx, nil)
			return err
		},
		FetchArtwork: func(context.Context, *pixiv.Client, int64) (pixiv.Artwork, error) {
			return pixiv.Artwork{ID: 42, Kind: pixiv.ArtworkKindIllustration}, nil
		},
	}
}
