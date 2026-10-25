package detail

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	downloadcmd "github.com/FlanChanXwO/pixiv-cli/internal/cli/download"
	commands "github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/command"
	pixivcmd "github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/pixivcmd"
	searchcmd "github.com/FlanChanXwO/pixiv-cli/internal/cli/search"
	downloader "github.com/FlanChanXwO/pixiv-cli/internal/media/downloader"
	recordpkg "github.com/FlanChanXwO/pixiv-cli/internal/record"
	"github.com/FlanChanXwO/pixiv-cli/internal/reversesearch"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
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
	cmd := newTestCommand(data)
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
	cmd := newTestCommand(data)
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
	artwork, err := recordpkg.RecordFromArtwork(pixiv.Artwork{
		ID: 501, Kind: pixiv.ArtworkKindIllustration, Title: "视觉作品",
	})
	requireNoError(t, err)

	var detailOutput bytes.Buffer
	detailCommand := newTestCommand(testMachineDependencies(
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
	downloadCommand := downloadcmd.New(
		commands.Env{In: strings.NewReader(detailOutput.String()), Out: &bytes.Buffer{}, Err: &bytes.Buffer{}, WrapUsage: func(err error) error { return err }},
		func(ctx context.Context, _ pixivcmd.Request, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
			_, err := attempt(ctx, &pixiv.Client{})
			return err
		},
		func() (downloadcmd.Runtime, error) { return downloadcmd.Runtime{}, nil },
		func() downloader.DownloadService {
			return downloader.DownloadService{
				NewBatchDownloader: func(_ downloader.DownloadClient, _, _ string) (downloader.BatchDownloader, error) {
					return captureBatchDownloader{ids: &downloadedIDs}, nil
				},
			}
		},
	)
	downloadCommand.SetArgs([]string{"--on-error", "fail-fast"})
	requireNoError(t, downloadCommand.Execute())
	if len(downloadedIDs) != 1 || downloadedIDs[0] != 501 {
		t.Fatalf("download consumer received IDs %v, want [501]", downloadedIDs)
	}
}

type captureBatchDownloader struct {
	ids *[]int64
}

func (m captureBatchDownloader) Download(_ context.Context, request downloader.DownloadRequest) (downloader.DownloadReport, error) {
	*m.ids = append(*m.ids, request.IllustIDs...)
	return downloader.DownloadReport{}, nil
}

func reverseSearchOutput(t *testing.T, results []reversesearch.Result, jsonOutput bool) string {
	t.Helper()
	var output bytes.Buffer
	cmd := searchcmd.New(
		commands.Env{In: strings.NewReader(""), Out: &output, Err: &bytes.Buffer{}, WrapUsage: func(err error) error { return err }, ResolveJSON: func(*bool) (bool, error) {
			return jsonOutput, nil
		}},
		nil,
		func(_ context.Context, request searchcmd.ReverseSearchRequest) (reversesearch.Response, error) {
			if request.Provider != reversesearch.ProviderAll {
				t.Fatalf("reverse provider = %q, want %q", request.Provider, reversesearch.ProviderAll)
			}
			return reversesearch.Response{Results: results}, nil
		},
	)
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
	data := testDependencies{
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

	cmd := newTestCommand(data)
	cmd.SetArgs([]string{"https://www.pixiv.net/users/7?secret=must-not-echo"})
	err := cmd.Execute()

	if sdk.ReasonOf(err) != sdk.InvalidArgument {
		t.Fatalf("expected unsupported URL error, got %v", err)
	}
	if opened {
		t.Fatal("client was opened before entity input validation")
	}
	if strings.Contains(err.Error(), "must-not-echo") {
		t.Fatal("unsupported URL query leaked into the error")
	}
}

func TestCommandNovelContentReturnsUnavailableBeforeOpeningClient(t *testing.T) {
	built := false
	opened := false
	data := testDependencies{
		Input:      strings.NewReader(""),
		Output:     &bytes.Buffer{},
		UsageError: func(err error) error { return err },
		BuildRequest: func(*cobra.Command, Options) (Request, error) {
			built = true
			return Request{}, nil
		},
		Pooled: func(context.Context, Request, func(context.Context, *pixiv.Client) (bool, error)) error {
			opened = true
			return nil
		},
		JSONOut: func(*bool) (bool, error) { return false, nil },
	}

	cmd := newTestCommand(data)
	cmd.SetArgs([]string{"--type", "novel", "--content", "42"})
	err := cmd.Execute()

	if sdk.ReasonOf(err) != sdk.ContentUnavailable {
		t.Fatalf("reason = %q, want %q (err=%v)", sdk.ReasonOf(err), sdk.ContentUnavailable, err)
	}
	if built {
		t.Fatal("request was built before reporting unavailable novel content")
	}
	if opened {
		t.Fatal("client was opened before reporting unavailable novel content")
	}
}

// 三种实体都走公开 SDK，且各自输出一个对应 key 的 JSON 文档；具体路由与字段映射由 SDK 测试负责。
func TestCommandTypedIDsOutputEntityEnvelopes(t *testing.T) {
	for _, typ := range []string{"artwork", "novel", "user"} {
		t.Run(typ, func(t *testing.T) {
			var output bytes.Buffer
			data := testMachineDependencies(&output, strings.NewReader(""))
			data.OutputIsTTY = func() bool { return true }
			cmd := newTestCommand(data)
			cmd.SetArgs([]string{"--type", typ, "42", "--json"})
			requireNoError(t, cmd.Execute())
			var object map[string]json.RawMessage
			if err := json.Unmarshal(output.Bytes(), &object); err != nil {
				t.Fatalf("expected one JSON object, got %q: %v", output.String(), err)
			}
			if len(object) == 0 {
				t.Fatalf("empty %s detail object: %q", typ, output.String())
			}
		})
	}
}

func TestCommandRejectsURLTypeConflictBeforeBuildingRequest(t *testing.T) {
	built := false
	opened := false
	cmd := newTestCommand(testDependencies{
		Input:      strings.NewReader(""),
		Output:     &bytes.Buffer{},
		UsageError: func(err error) error { return err },
		BuildRequest: func(*cobra.Command, Options) (Request, error) {
			built = true
			return Request{}, nil
		},
		Pooled: func(context.Context, Request, func(context.Context, *pixiv.Client) (bool, error)) error {
			opened = true
			return nil
		},
		JSONOut: func(*bool) (bool, error) { return false, nil },
	})
	cmd.SetArgs([]string{"--type", "novel", "https://www.pixiv.net/artworks/42?secret=must-not-echo"})

	err := cmd.Execute()
	if sdk.ReasonOf(err) != sdk.InvalidArgument {
		t.Fatalf("reason = %q, want %q (err=%v)", sdk.ReasonOf(err), sdk.InvalidArgument, err)
	}
	if built || opened {
		t.Fatal("URL/type conflict reached request construction or client execution")
	}
	if strings.Contains(err.Error(), "must-not-echo") {
		t.Fatal("URL query leaked into the conflict error")
	}
}

func TestCommandRejectsContentForNonNovelBeforeOpeningClient(t *testing.T) {
	opened := false
	data := testDependencies{
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

	cmd := newTestCommand(data)
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

			cmd := newTestCommand(data)
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

func TestCommandConsumesNormalSearchArtworkRecordsInOrder(t *testing.T) {
	artworkRecord := func(value pixiv.Artwork) recordpkg.Record {
		recordValue, err := recordpkg.RecordFromArtwork(value)
		requireNoError(t, err)
		return recordValue
	}
	records := []recordpkg.Record{
		artworkRecord(pixiv.Artwork{ID: 101, Kind: pixiv.ArtworkKindIllustration, Title: "作品一"}),
		artworkRecord(pixiv.Artwork{ID: 102, Kind: pixiv.ArtworkKindIllustration, Title: "作品二"}),
	}

	var output bytes.Buffer
	data := testMachineDependencies(&output, strings.NewReader(normalSearchRecordInput(t, records)))
	cmd := newTestCommand(data)
	cmd.SetArgs([]string{"--ndjson"})
	requireNoError(t, cmd.Execute())

	got := decodeMachineRecords(t, output.String())
	assertMachineRecordIDs(t, got, []string{"101", "102"})
	for index, value := range got {
		if value["type"] != "illust" {
			t.Fatalf("record %d has type %v, want illust: %#v", index, value["type"], value)
		}
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

func TestCommandRejectsJSONAndNDJSONTogether(t *testing.T) {
	data := testRecordDependencies(&bytes.Buffer{}, strings.NewReader(`{"id":"42","type":"illust","url":"https://www.pixiv.net/artworks/42"}
`))
	cmd := newTestCommand(data)
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
	cmd := newTestCommand(data)
	cmd.SetArgs([]string{"--ndjson", "--type", "novel"})
	if err := cmd.Execute(); err == nil || !strings.Contains(diagnostics.String(), "incompatible") {
		t.Fatalf("expected type mismatch, got err=%v diagnostics=%s", err, diagnostics.String())
	}
}

// 文本输入与 record 输入在 TTY/非 TTY 下的输出外形是 detail 自己的契约；
// 实体字段内容由 SDK/record 层覆盖。
func TestCommandOutputModes(t *testing.T) {
	twoRecords := `{"id":"42","type":"illust","url":"https://www.pixiv.net/artworks/42"}
{"id":"43","type":"manga","url":"https://www.pixiv.net/artworks/43"}
`
	for _, test := range []struct {
		name   string
		input  string
		args   []string
		tty    bool
		verify func(*testing.T, string)
	}{
		{
			name: "text value stays human off-TTY", input: "", args: []string{"42"}, tty: false,
			verify: func(t *testing.T, out string) {
				if !strings.Contains(out, "title: artwork") {
					t.Fatalf("expected human detail output, got %q", out)
				}
				if trimmed := strings.TrimSpace(out); strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
					t.Fatalf("text-value output unexpectedly became machine JSON: %q", out)
				}
			},
		},
		{
			name: "raw URL", input: "", args: []string{"https://www.pixiv.net/artworks/42"}, tty: false,
			verify: func(t *testing.T, out string) {
				if !strings.Contains(out, "title: artwork") {
					t.Fatalf("expected human detail output, got %q", out)
				}
			},
		},
		{
			name: "stdin ID", input: "42\n", tty: false,
			verify: func(t *testing.T, out string) {
				if !strings.Contains(out, "title: artwork") {
					t.Fatalf("expected human detail output, got %q", out)
				}
			},
		},
		{
			name: "text value NDJSON overrides TTY", input: "", args: []string{"42", "--ndjson"}, tty: true,
			verify: func(t *testing.T, out string) {
				records := decodeMachineRecords(t, out)
				if len(records) != 1 || records[0]["id"] != "42" || records[0]["type"] != "illust" {
					t.Fatalf("unexpected text-value canonical output: %q", out)
				}
			},
		},
		{
			name: "record TTY separates entries", input: twoRecords, tty: true,
			verify: func(t *testing.T, out string) {
				if strings.Count(out, "title: artwork") != 2 {
					t.Fatalf("expected two human detail entries, got %q", out)
				}
				if !strings.Contains(out, "\n---\n") {
					t.Fatalf("expected stable separator between TTY record entries, got %q", out)
				}
			},
		},
		{
			name: "record non-TTY auto NDJSON", input: twoRecords, tty: false,
			verify: func(t *testing.T, out string) {
				assertMachineRecordIDs(t, decodeMachineRecords(t, out), []string{"42", "43"})
			},
		},
		{
			name: "record explicit NDJSON", input: twoRecords, args: []string{"--ndjson"}, tty: true,
			verify: func(t *testing.T, out string) {
				assertMachineRecordIDs(t, decodeMachineRecords(t, out), []string{"42", "43"})
			},
		},
		{
			name: "record JSON array", input: twoRecords, args: []string{"--json"}, tty: true,
			verify: func(t *testing.T, out string) {
				var records []map[string]any
				if err := json.Unmarshal([]byte(out), &records); err != nil {
					t.Fatalf("expected one JSON array document, got %q: %v", out, err)
				}
				assertMachineRecordIDs(t, records, []string{"42", "43"})
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			data := testMachineDependencies(&output, strings.NewReader(test.input))
			data.OutputIsTTY = func() bool { return test.tty }
			cmd := newTestCommand(data)
			args := test.args
			if args == nil {
				args = []string{}
			}
			cmd.SetArgs(args)
			requireNoError(t, cmd.Execute())
			test.verify(t, output.String())
		})
	}
}

func TestCommandRecordMachineOutputUsesDetailTypes(t *testing.T) {
	// artwork 的 illust/manga/ugoira 别名归 record 与 SDK；这里只验证 detail 自己拼的 novel/user 记录。
	tests := []struct {
		name     string
		input    string
		wantID   string
		wantType string
	}{
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
			cmd := newTestCommand(data)
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

	cmd := newTestCommand(data)
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

func TestCommandNovelContentRecordModeReturnsUnavailableBeforeFetching(t *testing.T) {
	const recordInput = `{"id":"88","type":"novel","url":"https://www.pixiv.net/novel/show.php?id=88"}` + "\n"
	fetched := false
	data := testMachineDependencies(&bytes.Buffer{}, strings.NewReader(recordInput))
	data.FetchNovelContent = func(_ context.Context, _ *pixiv.Client, _ int64) (pixiv.NovelContent, error) {
		fetched = true
		return pixiv.NovelContent{}, nil
	}

	cmd := newTestCommand(data)
	cmd.SetArgs([]string{"--content", "--ndjson"})
	err := cmd.Execute()
	if sdk.ReasonOf(err) != sdk.ContentUnavailable {
		t.Fatalf("reason = %q, want %q (err=%v)", sdk.ReasonOf(err), sdk.ContentUnavailable, err)
	}
	if fetched {
		t.Fatal("novel content fetcher was called for an unsupported endpoint")
	}
}

func TestCommandRecordDiagnosticsStayOffStdout(t *testing.T) {
	var output bytes.Buffer
	var diagnostics bytes.Buffer
	data := testMachineDependencies(&output, strings.NewReader(`{"id":"42","type":"illust","url":"https://www.pixiv.net/artworks/42"}
not-json
`))
	data.ErrorOutput = &diagnostics
	cmd := newTestCommand(data)
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

	cmd := newTestCommand(data)
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
	cmd := newTestCommand(data)
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

func testMachineDependencies(output io.Writer, input io.Reader) testDependencies {
	return testDependencies{
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

func testRecordDependencies(output io.Writer, input io.Reader) testDependencies {
	return testDependencies{
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

// testDependencies preserves the previous test fields so existing assertions
// stay unchanged while production takes explicit arguments.
type testDependencies struct {
	Input             io.Reader
	Output            io.Writer
	ErrorOutput       io.Writer
	UsageError        func(error) error
	JSONOut           func(*bool) (bool, error)
	Pooled            func(context.Context, pixivcmd.Request, func(context.Context, *pixiv.Client) (bool, error)) error
	BuildRequest      func(*cobra.Command, Options) (pixivcmd.Request, error)
	FetchArtwork      func(context.Context, *pixiv.Client, int64) (pixiv.Artwork, error)
	FetchNovel        func(context.Context, *pixiv.Client, int64) (pixiv.Novel, error)
	FetchNovelContent func(context.Context, *pixiv.Client, int64) (pixiv.NovelContent, error)
	FetchUser         func(context.Context, *pixiv.Client, int64) (pixiv.UserDetail, error)
	OutputIsTTY       func() bool
}

type Request = pixivcmd.Request

func newTestCommand(d testDependencies) *cobra.Command {
	env := commands.Env{In: d.Input, Out: d.Output, Err: d.ErrorOutput, WrapUsage: d.UsageError, ResolveJSON: d.JSONOut}
	return newCommand(env, pixivcmd.Executor(d.Pooled), fetchers{
		artwork:      d.FetchArtwork,
		novel:        d.FetchNovel,
		novelContent: d.FetchNovelContent,
		user:         d.FetchUser,
	}, d.BuildRequest, d.OutputIsTTY)
}
