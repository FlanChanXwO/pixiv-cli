package detail

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	downloadcmd "github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/pixiv/download"
	searchcmd "github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/pixiv/search"
	downloader "github.com/FlanChanXwO/pixiv-cli/internal/media/downloader"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/reversesearch"
	recordpkg "github.com/FlanChanXwO/pixiv-cli/internal/shared/record"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/spf13/cobra"
)

func TestCommandConsumesReverseSearchIdentityRecordsInOrder(t *testing.T) {
	input := reverseSearchOutput(t, []reversesearch.Result{
		{Pixiv: &reversesearch.PixivRef{Type: reversesearch.PixivRefArtwork, ID: 401}},
		{Pixiv: &reversesearch.PixivRef{Type: reversesearch.PixivRefUser, ID: 402}},
		{Pixiv: &reversesearch.PixivRef{Type: reversesearch.PixivRefArtwork, ID: 403}},
	}, false)

	sourceRecords := decodeMachineRecords(t, input)
	assertMachineRecordIDs(t, sourceRecords, []string{"401", "402", "403"})
	for index, wantType := range []string{"artwork", "user", "artwork"} {
		if sourceRecords[index]["type"] != wantType {
			t.Fatalf("reverse record %d has type %v, want %s", index, sourceRecords[index]["type"], wantType)
		}
	}

	var output bytes.Buffer
	data := testMachineDependencies(&output, strings.NewReader(input))
	cmd := New(data)
	cmd.SetArgs([]string{"--ndjson"})
	requireNoError(t, cmd.Execute())

	got := decodeMachineRecords(t, output.String())
	assertMachineRecordIDs(t, got, []string{"401", "402", "403"})
	for index, wantType := range []string{"illust", "user", "illust"} {
		if got[index]["type"] != wantType {
			t.Fatalf("detail record %d has type %v, want %s", index, got[index]["type"], wantType)
		}
	}
}

func TestCommandRejectsReverseUserRecordForArtworkConstraintBeforeFetching(t *testing.T) {
	input := reverseSearchOutput(t, []reversesearch.Result{
		{Pixiv: &reversesearch.PixivRef{Type: reversesearch.PixivRefUser, ID: 402}},
	}, false)
	var output, diagnostics bytes.Buffer
	data := testMachineDependencies(&output, strings.NewReader(input))
	data.ErrorOutput = &diagnostics
	fetched := false
	data.Pooled = func(context.Context, Request, func(context.Context, *pixiv.Client) (bool, error)) error {
		fetched = true
		return nil
	}

	cmd := New(data)
	cmd.SetArgs([]string{"--type", "artwork", "--ndjson"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected reverse user record to be rejected for artwork constraint")
	}
	if !strings.Contains(diagnostics.String(), "incompatible") {
		t.Fatalf("expected type mismatch diagnostic, got %q", diagnostics.String())
	}
	if fetched {
		t.Fatal("opened client for a record rejected by explicit type constraint")
	}
}

func TestCommandDoesNotSplitReverseSearchAggregateJSONIntoRecords(t *testing.T) {
	aggregate := reverseSearchOutput(t, []reversesearch.Result{
		{Pixiv: &reversesearch.PixivRef{Type: reversesearch.PixivRefArtwork, ID: 404}},
	}, true)
	if !strings.Contains(aggregate, `"records"`) {
		t.Fatalf("expected reverse aggregate JSON to contain records, got %q", aggregate)
	}

	var output, diagnostics bytes.Buffer
	data := testMachineDependencies(&output, strings.NewReader(aggregate))
	data.ErrorOutput = &diagnostics
	fetched := false
	data.Pooled = func(context.Context, Request, func(context.Context, *pixiv.Client) (bool, error)) error {
		fetched = true
		return nil
	}
	cmd := New(data)
	cmd.SetArgs([]string{"--ndjson"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected aggregate reverse JSON to be rejected as a record stream")
	}
	if !strings.Contains(diagnostics.String(), `"code":"invalid_record"`) {
		t.Fatalf("expected structured invalid-record diagnostic, got %q", diagnostics.String())
	}
	if fetched {
		t.Fatal("opened client while parsing an aggregate reverse-search envelope")
	}
}

func TestDetailNDJSONFeedsExistingVisualDownloadConsumer(t *testing.T) {
	artwork, err := recordpkg.RecordFromArtworkDTO(pixiv.ToArtworkDTO(pixiv.Artwork{
		ID: 501, Kind: pixiv.ArtworkKindIllustration, Title: "视觉作品",
	}))
	requireNoError(t, err)

	var detailOutput bytes.Buffer
	detailCommand := New(testMachineDependencies(
		&detailOutput,
		strings.NewReader(normalSearchRecordInput(t, []recordpkg.Record{artwork})),
	))
	detailCommand.SetArgs([]string{"--ndjson"})
	requireNoError(t, detailCommand.Execute())
	detailRecords := decodeMachineRecords(t, detailOutput.String())
	assertMachineRecordIDs(t, detailRecords, []string{"501"})
	if detailRecords[0]["type"] != "illust" {
		t.Fatalf("detail output type = %v, want illust", detailRecords[0]["type"])
	}

	var downloadedIDs []int64
	downloadCommand := downloadcmd.New(downloadcmd.Deps{
		Input:       strings.NewReader(detailOutput.String()),
		Output:      &bytes.Buffer{},
		ErrorOutput: &bytes.Buffer{},
		UsageError:  func(err error) error { return err },
		Runtime: func() (downloadcmd.Runtime, error) {
			return downloadcmd.Runtime{}, nil
		},
		Pooled: func(ctx context.Context, _ downloadcmd.CommandRequest, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
			_, err := attempt(ctx, &pixiv.Client{})
			return err
		},
		Download: func() downloader.DownloadService {
			return downloader.DownloadService{
				NewManager: func(_ downloader.DownloadClient, _, _ string) (downloader.DownloadManager, error) {
					return captureDownloadManager{ids: &downloadedIDs}, nil
				},
			}
		},
	})
	downloadCommand.SetArgs([]string{"--on-error", "fail-fast"})
	requireNoError(t, downloadCommand.Execute())
	if len(downloadedIDs) != 1 || downloadedIDs[0] != 501 {
		t.Fatalf("download consumer received IDs %v, want [501]", downloadedIDs)
	}
}

type captureDownloadManager struct {
	ids *[]int64
}

func (m captureDownloadManager) Download(_ context.Context, request downloader.DownloadRequest) ([]downloader.DownloadedArtwork, error) {
	*m.ids = append(*m.ids, request.IllustIDs...)
	return nil, nil
}

func reverseSearchOutput(t *testing.T, results []reversesearch.Result, jsonOutput bool) string {
	t.Helper()
	var output bytes.Buffer
	cmd := searchcmd.New(searchcmd.Dependencies{
		Input:       strings.NewReader(""),
		Output:      &output,
		ErrorOutput: &bytes.Buffer{},
		UsageError:  func(err error) error { return err },
		JSONOut: func(*bool) (bool, error) {
			return jsonOutput, nil
		},
		ReverseSearch: func(_ context.Context, request searchcmd.ReverseSearchRequest) (reversesearch.Response, error) {
			if request.Provider != reversesearch.ProviderAll {
				t.Fatalf("reverse provider = %q, want %q", request.Provider, reversesearch.ProviderAll)
			}
			return reversesearch.Response{Results: results}, nil
		},
	})
	args := []string{"https://example.test/image.jpg", "--provider", "all"}
	if jsonOutput {
		args = append(args, "--json")
	} else {
		args = append(args, "--ndjson")
	}
	cmd.SetArgs(args)
	requireNoError(t, cmd.Execute())
	return output.String()
}

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

func TestCommandRejectsContentForNonNovelRecordBeforeFetching(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "artwork",
			input: `{"id":"42","type":"illust","url":"https://www.pixiv.net/artworks/42"}` + "\n",
		},
		{
			name:  "user",
			input: `{"id":"99","type":"user","url":"https://www.pixiv.net/users/99"}` + "\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			var diagnostics bytes.Buffer
			data := testMachineDependencies(&output, strings.NewReader(test.input))
			data.ErrorOutput = &diagnostics
			fetched := false
			data.FetchArtwork = func(_ context.Context, _ *pixiv.Client, id int64) (pixiv.Artwork, error) {
				fetched = true
				return pixiv.Artwork{ID: id, Kind: pixiv.ArtworkKindIllustration}, nil
			}
			data.FetchUser = func(_ context.Context, _ *pixiv.Client, id int64) (pixiv.UserDetail, error) {
				fetched = true
				return pixiv.UserDetail{User: pixiv.User{ID: id}}, nil
			}

			cmd := New(data)
			cmd.SetArgs([]string{"--content", "--ndjson"})
			err := cmd.Execute()
			if err == nil {
				t.Fatal("expected explicit non-novel content validation error")
			}
			if !strings.Contains(diagnostics.String(), "--content is only supported when --type novel") {
				t.Fatalf("expected explicit content validation diagnostic, got %q", diagnostics.String())
			}
			if fetched {
				t.Fatal("non-novel record was fetched despite --content validation failure")
			}
		})
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

func TestCommandConsumesNormalSearchCanonicalRecordsInOrder(t *testing.T) {
	artworkRecord := func(value pixiv.Artwork) recordpkg.Record {
		recordValue, err := recordpkg.RecordFromArtworkDTO(pixiv.ToArtworkDTO(value))
		requireNoError(t, err)
		return recordValue
	}
	novelRecord := func(value pixiv.Novel) recordpkg.Record {
		recordValue, err := recordpkg.RecordFromNovelDTO(pixiv.ToNovelDTO(value))
		requireNoError(t, err)
		return recordValue
	}
	userRecord := func(value pixiv.UserPreview) recordpkg.Record {
		recordValue, err := recordpkg.RecordFromUserPreviewDTO(pixiv.ToUserPreviewDTO(value))
		requireNoError(t, err)
		return recordValue
	}

	tests := []struct {
		name      string
		records   []recordpkg.Record
		wantIDs   []string
		wantTypes []string
	}{
		{
			name: "artwork search",
			records: []recordpkg.Record{
				artworkRecord(pixiv.Artwork{ID: 101, Kind: pixiv.ArtworkKindIllustration, Title: "作品一"}),
				artworkRecord(pixiv.Artwork{ID: 102, Kind: pixiv.ArtworkKindIllustration, Title: "作品二"}),
			},
			wantIDs:   []string{"101", "102"},
			wantTypes: []string{"illust", "illust"},
		},
		{
			name: "novel search",
			records: []recordpkg.Record{
				novelRecord(pixiv.Novel{ID: 201, Title: "小说一"}),
				novelRecord(pixiv.Novel{ID: 202, Title: "小说二"}),
			},
			wantIDs:   []string{"201", "202"},
			wantTypes: []string{"novel", "novel"},
		},
		{
			name: "user search preview",
			records: []recordpkg.Record{
				userRecord(pixiv.UserPreview{User: pixiv.User{ID: 301, Name: "用户一"}}),
				userRecord(pixiv.UserPreview{User: pixiv.User{ID: 302, Name: "用户二"}}),
			},
			wantIDs:   []string{"301", "302"},
			wantTypes: []string{"user", "user"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			data := testMachineDependencies(&output, strings.NewReader(normalSearchRecordInput(t, test.records)))
			cmd := New(data)
			cmd.SetArgs([]string{"--ndjson"})
			requireNoError(t, cmd.Execute())

			got := decodeMachineRecords(t, output.String())
			assertMachineRecordIDs(t, got, test.wantIDs)
			for index, wantType := range test.wantTypes {
				if got[index]["type"] != wantType {
					t.Fatalf("record %d has type %v, want %s: %#v", index, got[index]["type"], wantType, got[index])
				}
			}
		})
	}
}

func normalSearchRecordInput(t *testing.T, records []recordpkg.Record) string {
	t.Helper()
	var input strings.Builder
	for _, value := range records {
		raw, err := json.Marshal(value)
		requireNoError(t, err)

		var fields map[string]any
		requireNoError(t, json.Unmarshal(raw, &fields))
		fields["search_metadata"] = map[string]any{
			"filter":      map[string]any{"word": "fixture"},
			"sort":        "date_desc",
			"page":        2,
			"limit":       len(records),
			"next_cursor": "fixture-next",
		}
		fields["future_dto_field"] = map[string]any{"accepted": true}

		raw, err = json.Marshal(fields)
		requireNoError(t, err)
		input.Write(raw)
		input.WriteByte('\n')
	}
	return input.String()
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

func TestCommandTextValueAcceptsRawURLAndStdin(t *testing.T) {
	tests := []struct {
		name  string
		args  []string
		input string
	}{
		{
			name: "raw Pixiv URL",
			args: []string{"https://www.pixiv.net/artworks/42"},
		},
		{
			name:  "stdin raw ID",
			input: "42\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var output bytes.Buffer
			cmd := New(testMachineDependencies(&output, strings.NewReader(tc.input)))
			cmd.SetArgs(tc.args)
			requireNoError(t, cmd.Execute())
			if !strings.Contains(output.String(), "title: artwork") {
				t.Fatalf("expected compatible human detail output, got %q", output.String())
			}
		})
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
			name:     "illust artwork alias",
			input:    `{"id":"43","type":"illust","url":"https://www.pixiv.net/artworks/43"}` + "\n",
			wantID:   "43",
			wantType: "illust",
		},
		{
			name:     "manga artwork alias",
			input:    `{"id":"44","type":"manga","url":"https://www.pixiv.net/artworks/44"}` + "\n",
			wantID:   "44",
			wantType: "illust",
		},
		{
			name:     "ugoira artwork alias",
			input:    `{"id":"45","type":"ugoira","url":"https://www.pixiv.net/artworks/45"}` + "\n",
			wantID:   "45",
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

func TestCommandRejectsUnsupportedRecordTypeBeforeFetching(t *testing.T) {
	var output, diagnostics bytes.Buffer
	data := testMachineDependencies(&output, strings.NewReader(`{"id":"42","type":"mystery","url":"https://www.pixiv.net/artworks/42"}
`))
	data.ErrorOutput = &diagnostics
	fetched := false
	data.FetchArtwork = func(_ context.Context, _ *pixiv.Client, id int64) (pixiv.Artwork, error) {
		fetched = true
		return pixiv.Artwork{ID: id, Kind: pixiv.ArtworkKindIllustration}, nil
	}
	data.FetchNovel = func(_ context.Context, _ *pixiv.Client, id int64) (pixiv.Novel, error) {
		fetched = true
		return pixiv.Novel{ID: id}, nil
	}
	data.FetchUser = func(_ context.Context, _ *pixiv.Client, id int64) (pixiv.UserDetail, error) {
		fetched = true
		return pixiv.UserDetail{User: pixiv.User{ID: id}}, nil
	}

	cmd := New(data)
	cmd.SetArgs([]string{"--ndjson"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected unsupported record type to fail")
	}
	if !strings.Contains(diagnostics.String(), `unsupported record type`) || !strings.Contains(diagnostics.String(), `mystery`) {
		t.Fatalf("expected unsupported-type diagnostic, got %q", diagnostics.String())
	}
	if fetched {
		t.Fatal("unsupported record type was fetched")
	}
}

func TestCommandNovelContentMachineOutputUsesCanonicalRecordProjection(t *testing.T) {
	const recordInput = `{"id":"88","type":"novel","url":"https://www.pixiv.net/novel/show.php?id=88"}` + "\n"
	tests := []struct {
		name      string
		args      []string
		input     string
		jsonArray bool
	}{
		{
			name:  "record mode ndjson",
			args:  []string{"--content", "--ndjson"},
			input: recordInput,
		},
		{
			name:      "record mode json array",
			args:      []string{"--content", "--json"},
			input:     recordInput,
			jsonArray: true,
		},
		{
			name:  "text value ndjson",
			args:  []string{"88", "--type", "novel", "--content", "--ndjson"},
			input: "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			data := testMachineDependencies(&output, strings.NewReader(test.input))
			data.FetchNovelContent = func(_ context.Context, _ *pixiv.Client, id int64) (pixiv.NovelContent, error) {
				return pixiv.NovelContent{
					NovelID: id,
					Title:   "正文标题",
					Caption: "正文说明",
					Blocks: []pixiv.NovelBlock{{
						Kind: pixiv.NovelBlockParagraph,
						Text: "第一段",
					}},
				}, nil
			}

			cmd := New(data)
			cmd.SetArgs(test.args)
			requireNoError(t, cmd.Execute())

			var records []map[string]any
			if test.jsonArray {
				if err := json.Unmarshal(output.Bytes(), &records); err != nil {
					t.Fatalf("expected one JSON array document, got %q: %v", output.String(), err)
				}
			} else {
				records = decodeMachineRecords(t, output.String())
			}
			if len(records) != 1 {
				t.Fatalf("expected one novel content record, got %q", output.String())
			}

			record := records[0]
			if record["id"] != "88" || record["type"] != "novel" || record["url"] != "https://www.pixiv.net/novel/show.php?id=88" {
				t.Fatalf("novel content lost canonical identity: %#v", record)
			}
			if record["novel_id"] != float64(88) || record["title"] != "正文标题" || record["caption"] != "正文说明" {
				t.Fatalf("novel content lost structured fields: %#v", record)
			}
			blocks, ok := record["blocks"].([]any)
			if !ok || len(blocks) != 1 {
				t.Fatalf("novel content blocks are not preserved: %#v", record["blocks"])
			}
			block, ok := blocks[0].(map[string]any)
			if !ok || block["kind"] != "paragraph" || block["text"] != "第一段" {
				t.Fatalf("novel content block is not preserved: %#v", blocks[0])
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

func TestCommandPreservesRemoteDetailError(t *testing.T) {
	var output, diagnostics bytes.Buffer
	const failureMessage = "remote artwork detail failed"
	data := testMachineDependencies(&output, strings.NewReader(`{"id":"42","type":"illust","url":"https://www.pixiv.net/artworks/42"}
`))
	data.ErrorOutput = &diagnostics
	data.FetchArtwork = func(_ context.Context, _ *pixiv.Client, _ int64) (pixiv.Artwork, error) {
		return pixiv.Artwork{}, errors.New(failureMessage)
	}

	cmd := New(data)
	cmd.SetArgs([]string{"--ndjson"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected remote detail failure")
	}
	if !strings.Contains(diagnostics.String(), failureMessage) {
		t.Fatalf("remote detail failure was not preserved in diagnostics: %q", diagnostics.String())
	}
	if output.Len() != 0 {
		t.Fatalf("remote detail failure polluted stdout: %q", output.String())
	}
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
