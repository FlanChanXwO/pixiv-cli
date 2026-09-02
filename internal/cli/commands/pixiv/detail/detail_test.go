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

func TestCommandTextValueKeepsHumanOutputWhenStdoutIsNotTTY(t *testing.T) {
	var output bytes.Buffer
	data := testMachineDependencies(&output, strings.NewReader(""))
	data.OutputIsTTY = func() bool { return false }

	cmd := New(data)
	cmd.SetArgs([]string{"42"})
	requireNoError(t, cmd.Execute())

	got := output.String()
	if !strings.Contains(got, "title: artwork") {
		t.Fatalf("expected human detail output, got %q", got)
	}
	trimmed := strings.TrimSpace(got)
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		t.Fatalf("text-value output unexpectedly became machine JSON: %q", got)
	}
}

func TestCommandTextValueJSONRemainsSingleDocument(t *testing.T) {
	var output bytes.Buffer
	data := testMachineDependencies(&output, strings.NewReader(""))
	cmd := New(data)
	cmd.SetArgs([]string{"42", "--json"})
	requireNoError(t, cmd.Execute())

	var object map[string]any
	if err := json.Unmarshal(output.Bytes(), &object); err != nil {
		t.Fatalf("expected one JSON object, got %q: %v", output.String(), err)
	}
	if len(object) == 0 {
		t.Fatalf("expected detail DTO fields, got %q", output.String())
	}
	if strings.HasPrefix(strings.TrimSpace(output.String()), "[") {
		t.Fatalf("text-value --json unexpectedly became an array: %q", output.String())
	}
}

func TestCommandTextValueNDJSONOutputsCanonicalRecord(t *testing.T) {
	var output bytes.Buffer
	data := testMachineDependencies(&output, strings.NewReader(""))
	data.OutputIsTTY = func() bool { return true }
	cmd := New(data)
	cmd.SetArgs([]string{"42", "--ndjson"})
	requireNoError(t, cmd.Execute())

	records := decodeMachineRecords(t, output.String())
	if len(records) != 1 || records[0]["id"] != "42" || records[0]["type"] != "illust" {
		t.Fatalf("unexpected text-value canonical output: %q", output.String())
	}
}

func TestCommandRecordTTYSeparatesHumanDetails(t *testing.T) {
	var output bytes.Buffer
	data := testMachineDependencies(&output, strings.NewReader(`{"id":"42","type":"illust","url":"https://www.pixiv.net/artworks/42"}
{"id":"43","type":"manga","url":"https://www.pixiv.net/artworks/43"}
`))
	data.OutputIsTTY = func() bool { return true }
	cmd := New(data)
	cmd.SetArgs([]string{})
	requireNoError(t, cmd.Execute())

	got := output.String()
	if strings.Count(got, "title: artwork") != 2 {
		t.Fatalf("expected two human detail entries, got %q", got)
	}
	if !strings.Contains(got, "\n---\n") {
		t.Fatalf("expected stable separator between TTY record entries, got %q", got)
	}
}

func TestCommandRecordNonTTYAutoNDJSONPreservesOrder(t *testing.T) {
	var output bytes.Buffer
	data := testMachineDependencies(&output, strings.NewReader(`{"id":"42","type":"illust","url":"https://www.pixiv.net/artworks/42"}
{"id":"43","type":"manga","url":"https://www.pixiv.net/artworks/43"}
`))
	data.OutputIsTTY = func() bool { return false }
	cmd := New(data)
	cmd.SetArgs([]string{})
	requireNoError(t, cmd.Execute())

	assertMachineRecordIDs(t, decodeMachineRecords(t, output.String()), []string{"42", "43"})
}

func TestCommandRecordExplicitNDJSONOverridesTTY(t *testing.T) {
	var output bytes.Buffer
	data := testMachineDependencies(&output, strings.NewReader(`{"id":"42","type":"illust","url":"https://www.pixiv.net/artworks/42"}
{"id":"43","type":"manga","url":"https://www.pixiv.net/artworks/43"}
`))
	data.OutputIsTTY = func() bool { return true }
	cmd := New(data)
	cmd.SetArgs([]string{"--ndjson"})
	requireNoError(t, cmd.Execute())

	assertMachineRecordIDs(t, decodeMachineRecords(t, output.String()), []string{"42", "43"})
}

func TestCommandRecordJSONOutputsOrderedArray(t *testing.T) {
	var output bytes.Buffer
	data := testMachineDependencies(&output, strings.NewReader(`{"id":"42","type":"illust","url":"https://www.pixiv.net/artworks/42"}
{"id":"43","type":"manga","url":"https://www.pixiv.net/artworks/43"}
`))
	data.OutputIsTTY = func() bool { return true }
	cmd := New(data)
	cmd.SetArgs([]string{"--json"})
	requireNoError(t, cmd.Execute())

	var records []map[string]any
	if err := json.Unmarshal(output.Bytes(), &records); err != nil {
		t.Fatalf("expected one JSON array document, got %q: %v", output.String(), err)
	}
	assertMachineRecordIDs(t, records, []string{"42", "43"})
}

func TestCommandRecordMachineOutputUsesDetailTypes(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantID   string
		wantType string
	}{
		{
			name:     "generic artwork becomes concrete artwork kind",
			input:    `{"id":"42","type":"artwork","url":"https://www.pixiv.net/artworks/42"}` + "\n",
			wantID:   "42",
			wantType: "illust",
		},
		{
			name:     "novel",
			input:    `{"id":"88","type":"novel","url":"https://www.pixiv.net/novel/show.php?id=88"}` + "\n",
			wantID:   "88",
			wantType: "novel",
		},
		{
			name:     "user",
			input:    `{"id":"99","type":"user","url":"https://www.pixiv.net/users/99"}` + "\n",
			wantID:   "99",
			wantType: "user",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			data := testMachineDependencies(&output, strings.NewReader(test.input))
			cmd := New(data)
			cmd.SetArgs([]string{"--ndjson"})
			requireNoError(t, cmd.Execute())

			records := decodeMachineRecords(t, output.String())
			if len(records) != 1 || records[0]["id"] != test.wantID || records[0]["type"] != test.wantType {
				t.Fatalf("unexpected detail record: %q", output.String())
			}
			if test.wantType == "user" {
				user, ok := records[0]["user"].(map[string]any)
				if !ok || user["name"] != "user" {
					t.Fatalf("expected detailed user envelope, got %q", output.String())
				}
			}
		})
	}
}

func TestCommandRecordDiagnosticsStayOffStdout(t *testing.T) {
	var output bytes.Buffer
	var diagnostics bytes.Buffer
	data := testMachineDependencies(&output, strings.NewReader(`{"id":"42","type":"illust","url":"https://www.pixiv.net/artworks/42"}
not-json
`))
	data.ErrorOutput = &diagnostics
	cmd := New(data)
	cmd.SetArgs([]string{"--ndjson"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected malformed record to fail")
	}

	if !strings.Contains(diagnostics.String(), `"code":"invalid_record"`) {
		t.Fatalf("expected structured record diagnostic on stderr, got %q", diagnostics.String())
	}
	if strings.Contains(output.String(), "invalid_record") {
		t.Fatalf("record diagnostic polluted stdout: %q", output.String())
	}
	assertMachineRecordIDs(t, decodeMachineRecords(t, output.String()), []string{"42"})
}

func TestCommandRecordOutputWriteErrorRemainsOriginal(t *testing.T) {
	data := testMachineDependencies(failingWriter{err: outputWriteError{}}, strings.NewReader(`{"id":"42","type":"illust","url":"https://www.pixiv.net/artworks/42"}
`))
	cmd := New(data)
	cmd.SetArgs([]string{"--ndjson"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "output write failed") {
		t.Fatalf("expected original output write error, got %v", err)
	}
}

type outputWriteError struct{}

func (outputWriteError) Error() string { return "output write failed" }

type failingWriter struct {
	err error
}

func (w failingWriter) Write([]byte) (int, error) { return 0, w.err }

func testMachineDependencies(output io.Writer, input io.Reader) Dependencies {
	return Dependencies{
		Input:       input,
		Output:      output,
		ErrorOutput: &bytes.Buffer{},
		UsageError:  func(err error) error { return err },
		BuildRequest: func(*cobra.Command, Options) (Request, error) {
			return Request{}, nil
		},
		JSONOut: func(override *bool) (bool, error) {
			return override != nil && *override, nil
		},
		Pooled: func(ctx context.Context, _ Request, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
			_, err := attempt(ctx, nil)
			return err
		},
		FetchArtwork: func(_ context.Context, _ *pixiv.Client, id int64) (pixiv.Artwork, error) {
			return pixiv.Artwork{ID: id, Kind: pixiv.ArtworkKindIllustration, Title: "artwork"}, nil
		},
		FetchNovel: func(_ context.Context, _ *pixiv.Client, id int64) (pixiv.Novel, error) {
			return pixiv.Novel{ID: id, Title: "novel"}, nil
		},
		FetchNovelContent: func(_ context.Context, _ *pixiv.Client, id int64) (pixiv.NovelContent, error) {
			return pixiv.NovelContent{NovelID: id, Title: "novel-content"}, nil
		},
		FetchUser: func(_ context.Context, _ *pixiv.Client, id int64) (pixiv.UserDetail, error) {
			return pixiv.UserDetail{User: pixiv.User{ID: id, Name: "user"}}, nil
		},
	}
}

func decodeMachineRecords(t *testing.T, output string) []map[string]any {
	t.Helper()
	var records []map[string]any
	for lineNumber, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var value map[string]any
		if err := json.Unmarshal([]byte(line), &value); err != nil {
			t.Fatalf("line %d is not a canonical JSON record: %q: %v", lineNumber+1, line, err)
		}
		records = append(records, value)
	}
	return records
}

func assertMachineRecordIDs(t *testing.T, records []map[string]any, want []string) {
	t.Helper()
	if len(records) != len(want) {
		t.Fatalf("expected %d records, got %d: %#v", len(want), len(records), records)
	}
	for index, expectedID := range want {
		if records[index]["id"] != expectedID {
			t.Fatalf("record %d has id %v, want %s", index, records[index]["id"], expectedID)
		}
		if records[index]["url"] == nil || records[index]["type"] == nil {
			t.Fatalf("record %d is missing canonical fields: %#v", index, records[index])
		}
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
