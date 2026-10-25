package bookmark

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	commands "github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/command"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/input"
	pixivcmd "github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/pixivcmd"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBookmarkHelpUsesTypedTargetNames(t *testing.T) {
	cmd := newTestCommand(testData{Input: strings.NewReader(""), Output: &bytes.Buffer{}, ErrorOutput: &bytes.Buffer{}, UsageError: func(err error) error { return err }})
	children := map[string]*cobra.Command{}
	for _, child := range cmd.Commands() {
		children[child.Name()] = child
	}

	if got := cmd.Short; got != "Manage artwork and novel bookmarks" {
		t.Fatalf("bookmark short = %q, want typed bookmark description", got)
	}
	checks := map[string]struct {
		use   string
		short string
	}{
		"list":   {use: "list [USER_ID_OR_URL]", short: "List artwork or novel bookmarks"},
		"tags":   {use: "tags [USER_ID_OR_URL]", short: "List artwork or novel bookmark tags"},
		"detail": {use: "detail ARTWORK_ID_OR_NOVEL_ID_OR_URL", short: "Show the current user's bookmark detail"},
	}
	for name, want := range checks {
		child, ok := children[name]
		if !ok {
			t.Fatalf("bookmark command missing %q", name)
		}
		if child.Use != want.use || child.Short != want.short {
			t.Fatalf("%s help = use %q/short %q, want use %q/short %q", name, child.Use, child.Short, want.use, want.short)
		}
	}
}

func TestBookmarkListAllUsesOneLogicalLimitAndTypedNDJSON(t *testing.T) {
	output := &bytes.Buffer{}
	var paths []string
	transport := bookmarkRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		paths = append(paths, request.URL.Path)
		if request.URL.Query().Get("user_id") != "401" || request.URL.Query().Get("restrict") != "public" {
			t.Fatalf("bookmark query = %v", request.URL.Query())
		}
		switch request.URL.Path {
		case "/v1/user/bookmarks/illust":
			return bookmarkJSONResponse(request, http.StatusOK, `{"illusts":[{"id":1001,"title":"artwork","type":"illust","create_date":"2024-05-01T10:00:00+09:00","user":{"id":7,"name":"artist"},"tags":[]}],"next_url":null}`), nil
		case "/v1/user/bookmarks/novel":
			return bookmarkJSONResponse(request, http.StatusOK, `{"novels":[{"id":2001,"title":"novel","create_date":"2024-05-01T10:00:00+09:00","user":{"id":7,"name":"writer"},"tags":[]}],"next_url":null}`), nil
		default:
			return nil, io.ErrUnexpectedEOF
		}
	})
	client, err := pixiv.NewWith("test-access-token", pixiv.Options{HTTPClient: &http.Client{Transport: transport}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}

	cmd := newTestCommand(testData{
		Input:       strings.NewReader(""),
		Output:      output,
		ErrorOutput: &bytes.Buffer{},
		UsageError:  func(err error) error { return err },
		JSONOut:     func(*bool) (bool, error) { return false, nil },
		Pooled: func(ctx context.Context, _ pixivcmd.Request, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
			_, err := attempt(ctx, client)
			return err
		},
	})
	cmd.SetArgs([]string{"list", "401", "--type", "all", "--limit", "2", "--ndjson"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(paths) != 2 || paths[0] != "/v1/user/bookmarks/illust" || paths[1] != "/v1/user/bookmarks/novel" {
		t.Fatalf("request paths = %v, want artwork then novel", paths)
	}
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("NDJSON lines = %d, want 2; output=%q", len(lines), output.String())
	}
	want := []struct {
		id   string
		kind string
		url  string
	}{
		{id: "1001", kind: "illust", url: "https://www.pixiv.net/artworks/1001"},
		{id: "2001", kind: "novel", url: "https://www.pixiv.net/novel/show.php?id=2001"},
	}
	for index, line := range lines {
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("decode NDJSON record %d: %v; output=%q", index, err, output.String())
		}
		if record["id"] != want[index].id || record["type"] != want[index].kind || record["url"] != want[index].url {
			t.Fatalf("record %d = %#v, want id=%q type=%q url=%q", index, record, want[index].id, want[index].kind, want[index].url)
		}
	}
}

func TestBookmarkTagsAllPreservesSameNameCountsAndType(t *testing.T) {
	output := &bytes.Buffer{}
	var paths []string
	transport := bookmarkRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		paths = append(paths, request.URL.Path)
		if request.URL.Query().Get("user_id") != "401" || request.URL.Query().Get("restrict") != "public" {
			t.Fatalf("bookmark tag query = %v", request.URL.Query())
		}
		switch request.URL.Path {
		case "/v1/user/bookmark-tags/illust":
			return bookmarkJSONResponse(request, http.StatusOK, `{"bookmark_tags":[{"name":"same","count":3}],"next_url":null}`), nil
		case "/v1/user/bookmark-tags/novel":
			return bookmarkJSONResponse(request, http.StatusOK, `{"bookmark_tags":[{"name":"same","count":7}],"next_url":null}`), nil
		default:
			return nil, io.ErrUnexpectedEOF
		}
	})
	client, err := pixiv.NewWith("test-access-token", pixiv.Options{HTTPClient: &http.Client{Transport: transport}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}

	cmd := newTestCommand(testData{
		Input:       strings.NewReader(""),
		Output:      output,
		ErrorOutput: &bytes.Buffer{},
		UsageError:  func(err error) error { return err },
		JSONOut:     func(*bool) (bool, error) { return true, nil },
		Pooled: func(ctx context.Context, _ pixivcmd.Request, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
			_, err := attempt(ctx, client)
			return err
		},
	})
	cmd.SetArgs([]string{"tags", "401", "--type", "all", "--limit", "0", "--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(paths) != 2 || paths[0] != "/v1/user/bookmark-tags/illust" || paths[1] != "/v1/user/bookmark-tags/novel" {
		t.Fatalf("request paths = %v, want artwork then novel", paths)
	}
	var envelope struct {
		Tags []struct {
			Name  string `json:"name"`
			Count int    `json:"count"`
			Type  string `json:"type"`
		} `json:"bookmark_tags"`
	}
	if err := json.Unmarshal(output.Bytes(), &envelope); err != nil {
		t.Fatalf("decode JSON envelope: %v; output=%q", err, output.String())
	}
	if len(envelope.Tags) != 2 {
		t.Fatalf("bookmark tags = %#v, want two typed entries", envelope.Tags)
	}
	want := []struct {
		name  string
		count int
		typ   string
	}{
		{name: "same", count: 3, typ: "artwork"},
		{name: "same", count: 7, typ: "novel"},
	}
	for index, tag := range envelope.Tags {
		if tag.Name != want[index].name || tag.Count != want[index].count || tag.Type != want[index].typ {
			t.Fatalf("tag %d = %#v, want %#v", index, tag, want[index])
		}
	}
}

func TestBookmarkListAcceptsUserURLForNovel(t *testing.T) {
	output := &bytes.Buffer{}
	transport := bookmarkRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path != "/v1/user/bookmarks/novel" {
			t.Fatalf("path = %q, want novel bookmarks", request.URL.Path)
		}
		if request.URL.Query().Get("user_id") != "401" {
			t.Fatalf("user_id = %q, want 401", request.URL.Query().Get("user_id"))
		}
		return bookmarkJSONResponse(request, http.StatusOK, `{"novels":[{"id":2001,"title":"novel","create_date":"2024-05-01T10:00:00+09:00","user":{"id":7,"name":"writer"},"tags":[]}],"next_url":null}`), nil
	})
	client, err := pixiv.NewWith("test-access-token", pixiv.Options{HTTPClient: &http.Client{Transport: transport}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}

	cmd := newTestCommand(testData{
		Input:       strings.NewReader(""),
		Output:      output,
		ErrorOutput: &bytes.Buffer{},
		UsageError:  func(err error) error { return err },
		JSONOut:     func(*bool) (bool, error) { return true, nil },
		Pooled: func(ctx context.Context, _ pixivcmd.Request, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
			_, err := attempt(ctx, client)
			return err
		},
	})
	cmd.SetArgs([]string{"list", "https://www.pixiv.net/users/401", "--type", "novel", "--limit", "1", "--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	var envelope struct {
		Novels []struct {
			ID int64 `json:"id"`
		} `json:"novels"`
	}
	if err := json.Unmarshal(output.Bytes(), &envelope); err != nil {
		t.Fatalf("decode JSON envelope: %v; output=%q", err, output.String())
	}
	if len(envelope.Novels) != 1 || envelope.Novels[0].ID != 2001 {
		t.Fatalf("novels = %#v, want novel 2001", envelope.Novels)
	}
}

func TestBookmarkOwnerCommandsRejectArtworkBookmarksURLForOtherTypesBeforeNetwork(t *testing.T) {
	for _, commandName := range []string{"list", "tags"} {
		for _, typ := range []string{"novel", "all"} {
			t.Run(commandName+"-"+typ, func(t *testing.T) {
				calls := 0
				cmd := newTestCommand(testData{
					Input:       strings.NewReader(""),
					Output:      &bytes.Buffer{},
					ErrorOutput: &bytes.Buffer{},
					UsageError:  func(err error) error { return err },
					Pooled: func(context.Context, pixivcmd.Request, func(context.Context, *pixiv.Client) (bool, error)) error {
						calls++
						return nil
					},
				})
				cmd.SetArgs([]string{
					commandName,
					"https://www.pixiv.net/users/401/bookmarks/artworks",
					"--type", typ,
					"--limit", "1",
					"--json",
				})

				err := cmd.Execute()
				if err == nil || !strings.Contains(err.Error(), "URL namespace conflicts with the selected type") {
					t.Fatalf("Execute error = %v, want URL namespace conflict", err)
				}
				if calls != 0 {
					t.Fatalf("Pooled called %d time(s), want no network call", calls)
				}
			})
		}
	}
}

func TestBookmarkDetailRejectsAllBeforeNetwork(t *testing.T) {
	calls := 0
	cmd := newTestCommand(testData{
		Input:       strings.NewReader(""),
		Output:      &bytes.Buffer{},
		ErrorOutput: &bytes.Buffer{},
		UsageError:  func(err error) error { return err },
		Pooled: func(context.Context, pixivcmd.Request, func(context.Context, *pixiv.Client) (bool, error)) error {
			calls++
			return nil
		},
	})
	cmd.SetArgs([]string{"detail", "9001", "--type", "all", "--json"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), `type "all" is not supported by this command`) {
		t.Fatalf("Execute error = %v, want unsupported all type", err)
	}
	if calls != 0 {
		t.Fatalf("Pooled called %d time(s), want no network call", calls)
	}
}

func TestBookmarkDetailSupportsNovelType(t *testing.T) {
	output := &bytes.Buffer{}
	transport := bookmarkRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		return bookmarkJSONResponse(request, http.StatusOK, `{"bookmark_detail":{"is_bookmarked":true,"restrict":"private","tags":[{"name":"same","is_registered":true}]}}`), nil
	})
	client, err := pixiv.NewWith("test-access-token", pixiv.Options{HTTPClient: &http.Client{Transport: transport}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}

	cmd := newTestCommand(testData{
		Input:       strings.NewReader(""),
		Output:      output,
		ErrorOutput: &bytes.Buffer{},
		UsageError:  func(err error) error { return err },
		JSONOut:     func(*bool) (bool, error) { return true, nil },
		Pooled: func(ctx context.Context, _ pixivcmd.Request, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
			_, err := attempt(ctx, client)
			return err
		},
	})
	cmd.SetArgs([]string{"detail", "9001", "--type", "novel", "--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	var detail struct {
		Restrict string   `json:"restrict"`
		Tags     []string `json:"tags"`
	}
	if err := json.Unmarshal(output.Bytes(), &detail); err != nil {
		t.Fatalf("decode JSON detail: %v; output=%q", err, output.String())
	}
	if detail.Restrict != "private" || len(detail.Tags) != 1 || detail.Tags[0] != "same" {
		t.Fatalf("detail = %#v, want private same", detail)
	}
}

func TestBookmarkTagsSupportsNovelType(t *testing.T) {
	output := &bytes.Buffer{}
	transport := bookmarkRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		return bookmarkJSONResponse(request, http.StatusOK, `{"bookmark_tags":[{"name":"story","count":4}],"next_url":null}`), nil
	})
	client, err := pixiv.NewWith("test-access-token", pixiv.Options{HTTPClient: &http.Client{Transport: transport}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}

	cmd := newTestCommand(testData{
		Input:       strings.NewReader(""),
		Output:      output,
		ErrorOutput: &bytes.Buffer{},
		UsageError:  func(err error) error { return err },
		JSONOut:     func(*bool) (bool, error) { return true, nil },
		Pooled: func(ctx context.Context, _ pixivcmd.Request, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
			_, err := attempt(ctx, client)
			return err
		},
	})
	cmd.SetArgs([]string{"tags", "401", "--type", "novel", "--limit", "1", "--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	var envelope struct {
		Tags []struct {
			Name  string `json:"name"`
			Count int    `json:"count"`
		} `json:"bookmark_tags"`
	}
	if err := json.Unmarshal(output.Bytes(), &envelope); err != nil {
		t.Fatalf("decode JSON envelope: %v; output=%q", err, output.String())
	}
	if len(envelope.Tags) != 1 || envelope.Tags[0].Name != "story" || envelope.Tags[0].Count != 4 {
		t.Fatalf("bookmark tags = %#v, want story/4", envelope.Tags)
	}
}

func TestBookmarkListAcceptsCanonicalUserRecordForNovel(t *testing.T) {
	output := &bytes.Buffer{}
	transport := bookmarkRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path != "/v1/user/bookmarks/novel" || request.URL.Query().Get("user_id") != "401" {
			t.Fatalf("bookmark request = %s?%s", request.URL.Path, request.URL.RawQuery)
		}
		return bookmarkJSONResponse(request, http.StatusOK, `{"novels":[{"id":2001,"title":"novel","create_date":"2024-05-01T10:00:00+09:00","user":{"id":7,"name":"writer"},"tags":[]}],"next_url":null}`), nil
	})
	client, err := pixiv.NewWith("test-access-token", pixiv.Options{HTTPClient: &http.Client{Transport: transport}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}

	cmd := newTestCommand(testData{
		Input:       strings.NewReader(`{"id":"401","type":"user","url":"https://www.pixiv.net/users/401"}` + "\n"),
		Output:      output,
		ErrorOutput: &bytes.Buffer{},
		UsageError:  func(err error) error { return err },
		JSONOut:     func(*bool) (bool, error) { return true, nil },
		Pooled: func(ctx context.Context, _ pixivcmd.Request, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
			_, err := attempt(ctx, client)
			return err
		},
	})
	cmd.SetArgs([]string{"list", "--type", "novel", "--limit", "1", "--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	var envelope struct {
		Novels []struct {
			ID int64 `json:"id"`
		} `json:"novels"`
	}
	if err := json.Unmarshal(output.Bytes(), &envelope); err != nil {
		t.Fatalf("decode JSON envelope: %v; output=%q", err, output.String())
	}
	if len(envelope.Novels) != 1 || envelope.Novels[0].ID != 2001 {
		t.Fatalf("novels = %#v, want novel 2001", envelope.Novels)
	}
}

// 聚合分支在第二个上游失败时不得提交任何 JSON/NDJSON；list 与 tags 共用同一提交边界。
func TestBookmarkAllFailureDoesNotCommitJSONOrNDJSON(t *testing.T) {
	for _, command := range []string{"list", "tags"} {
		for _, outputMode := range []string{"json", "ndjson"} {
			t.Run(command+"-"+outputMode, func(t *testing.T) {
				output := &bytes.Buffer{}
				calls := 0
				artworkPath, novelPath := "/v1/user/bookmarks/illust", "/v1/user/bookmarks/novel"
				artworkBody := `{"illusts":[{"id":1001,"title":"artwork","type":"illust","create_date":"2024-05-01T10:00:00+09:00","user":{"id":7,"name":"artist"},"tags":[]}],"next_url":null}`
				if command == "tags" {
					artworkPath, novelPath = "/v1/user/bookmark-tags/illust", "/v1/user/bookmark-tags/novel"
					artworkBody = `{"bookmark_tags":[{"name":"artwork","count":3}],"next_url":null}`
				}
				transport := bookmarkRoundTripFunc(func(request *http.Request) (*http.Response, error) {
					calls++
					switch request.URL.Path {
					case artworkPath:
						return bookmarkJSONResponse(request, http.StatusOK, artworkBody), nil
					case novelPath:
						return bookmarkJSONResponse(request, http.StatusInternalServerError, `{"error":"novel upstream failed"}`), nil
					default:
						return nil, io.ErrUnexpectedEOF
					}
				})
				client, err := pixiv.NewWith("test-access-token", pixiv.Options{HTTPClient: &http.Client{Transport: transport}})
				if err != nil {
					t.Fatalf("NewWith: %v", err)
				}

				cmd := newTestCommand(testData{
					Input:       strings.NewReader(""),
					Output:      output,
					ErrorOutput: &bytes.Buffer{},
					UsageError:  func(err error) error { return err },
					JSONOut:     func(*bool) (bool, error) { return outputMode == "json", nil },
					Pooled: func(ctx context.Context, _ pixivcmd.Request, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
						_, err := attempt(ctx, client)
						return err
					},
				})
				args := []string{command, "401", "--type", "all", "--limit", "0", "--" + outputMode}
				cmd.SetArgs(args)

				if err := cmd.Execute(); err == nil {
					t.Fatal("Execute unexpectedly succeeded")
				}
				if calls != 2 {
					t.Fatalf("requests = %d, want artwork then novel", calls)
				}
				if output.Len() != 0 {
					t.Fatalf("output committed after aggregate failure: %q", output.String())
				}
			})
		}
	}
}

func TestBookmarkAllResolvesCurrentUserPerPoolAttempt(t *testing.T) {
	for _, commandName := range []string{"list", "tags"} {
		t.Run(commandName, func(t *testing.T) {
			first := bookmarkOpenClient(t, 11, func(request *http.Request) (*http.Response, error) {
				if got := request.URL.Query().Get("user_id"); got != "11" {
					t.Fatalf("first attempt user_id = %q, want 11", got)
				}
				return bookmarkPoolReplayResponse(request, commandName, true), nil
			})
			second := bookmarkOpenClient(t, 22, func(request *http.Request) (*http.Response, error) {
				if got := request.URL.Query().Get("user_id"); got != "22" {
					t.Fatalf("replayed attempt user_id = %q, want 22", got)
				}
				return bookmarkPoolReplayResponse(request, commandName, false), nil
			})
			output := &bytes.Buffer{}
			cmd := newTestCommand(testData{
				Input:       strings.NewReader(""),
				Output:      output,
				ErrorOutput: &bytes.Buffer{},
				UsageError:  func(err error) error { return err },
				JSONOut:     func(*bool) (bool, error) { return true, nil },
				Pooled: func(ctx context.Context, _ pixivcmd.Request, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
					committed, err := attempt(ctx, first)
					if err == nil || committed {
						return err
					}
					_, err = attempt(ctx, second)
					return err
				},
			})
			cmd.SetArgs([]string{commandName, "--type", "all", "--limit", "0", "--json"})

			if err := cmd.Execute(); err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if output.Len() == 0 {
				t.Fatal("replayed aggregate produced no output")
			}
			if commandName == "list" && !strings.Contains(output.String(), "2002") {
				t.Fatalf("replayed list output = %q, want second-account novel", output.String())
			}
			if commandName == "tags" && !strings.Contains(output.String(), "novel-tag") {
				t.Fatalf("replayed tags output = %q, want second-account novel tag", output.String())
			}
		})
	}
}

func bookmarkOpenClient(t *testing.T, userID int64, handler bookmarkRoundTripFunc) *pixiv.Client {
	t.Helper()
	transport := bookmarkRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Host == "oauth.secure.pixiv.net" {
			return bookmarkJSONResponse(request, http.StatusOK, fmt.Sprintf(`{"access_token":"access-%d","refresh_token":"refresh-%d","expires_in":3600,"user":{"id":%d}}`, userID, userID, userID)), nil
		}
		return handler(request)
	})
	client, _, err := pixiv.OpenWith(context.Background(), "refresh-token", pixiv.Options{HTTPClient: &http.Client{Transport: transport}})
	if err != nil {
		t.Fatalf("OpenWith: %v", err)
	}
	return client
}

func bookmarkPoolReplayResponse(request *http.Request, commandName string, failNovel bool) *http.Response {
	if commandName == "list" {
		switch request.URL.Path {
		case "/v1/user/bookmarks/illust":
			return bookmarkJSONResponse(request, http.StatusOK, `{"illusts":[{"id":1002,"title":"artwork","type":"illust","create_date":"2024-05-01T10:00:00+09:00","user":{"id":7,"name":"artist"},"tags":[]}],"next_url":null}`)
		case "/v1/user/bookmarks/novel":
			if failNovel {
				return bookmarkJSONResponse(request, http.StatusInternalServerError, `{"error":"novel upstream failed"}`)
			}
			return bookmarkJSONResponse(request, http.StatusOK, `{"novels":[{"id":2002,"title":"novel","create_date":"2024-05-01T10:00:00+09:00","user":{"id":7,"name":"writer"},"tags":[]}],"next_url":null}`)
		}
	}
	if commandName == "tags" {
		switch request.URL.Path {
		case "/v1/user/bookmark-tags/illust":
			return bookmarkJSONResponse(request, http.StatusOK, `{"bookmark_tags":[{"name":"artwork-tag","count":3}],"next_url":null}`)
		case "/v1/user/bookmark-tags/novel":
			if failNovel {
				return bookmarkJSONResponse(request, http.StatusInternalServerError, `{"error":"novel upstream failed"}`)
			}
			return bookmarkJSONResponse(request, http.StatusOK, `{"bookmark_tags":[{"name":"novel-tag","count":7}],"next_url":null}`)
		}
	}
	return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader("unexpected bookmark replay request")), Request: request}
}

type bookmarkRoundTripFunc func(*http.Request) (*http.Response, error)

func (f bookmarkRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func bookmarkJSONResponse(request *http.Request, status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    request,
	}
}

// frozen cli-migration-matrix 将 bookmark detail/add/remove 冻结为 artwork 或
// novel 双 namespace；add/remove 必须与 detail 一致地按 --type 分发到 novel
// bookmark mutation，且 URL/record/显式类型不得跨 namespace。
func TestBookmarkMutationsSupportNovelType(t *testing.T) {
	for _, tt := range []struct {
		command string
		args    []string
		form    map[string]string
	}{
		{command: "add", args: []string{"add", "9001", "--type", "novel", "--restrict", "private"}, form: map[string]string{"novel_id": "9001", "restrict": "private"}},
		{command: "remove", args: []string{"remove", "9001", "--type", "novel"}, form: map[string]string{"novel_id": "9001"}},
	} {
		t.Run(tt.command, func(t *testing.T) {
			called := 0
			transport := bookmarkRoundTripFunc(func(request *http.Request) (*http.Response, error) {
				called++
				if err := request.ParseForm(); err != nil {
					t.Fatalf("ParseForm: %v", err)
				}
				for key, want := range tt.form {
					if got := request.PostForm.Get(key); got != want {
						t.Fatalf("%s = %q, want %q", key, got, want)
					}
				}
				return bookmarkJSONResponse(request, http.StatusOK, `{}`), nil
			})
			client, err := pixiv.NewWith("test-access-token", pixiv.Options{HTTPClient: &http.Client{Transport: transport}})
			if err != nil {
				t.Fatalf("NewWith: %v", err)
			}
			cmd := newTestCommand(testData{
				Input: strings.NewReader(""), Output: &bytes.Buffer{}, ErrorOutput: &bytes.Buffer{},
				UsageError: func(err error) error { return err },
				Pooled: func(ctx context.Context, _ pixivcmd.Request, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
					_, err := attempt(ctx, client)
					return err
				},
			})
			cmd.SetArgs(tt.args)
			if err := cmd.Execute(); err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if called != 1 {
				t.Fatalf("novel %s requests = %d, want 1", tt.command, called)
			}
		})
	}
}

func TestBookmarkMutationRejectsUnsupportedTypeBeforeNetwork(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
	}{
		{name: "add all", args: []string{"add", "9001", "--type", "all"}},
		{name: "add user", args: []string{"add", "9001", "--type", "user"}},
		{name: "remove all", args: []string{"remove", "9001", "--type", "all"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			pooledCalls := 0
			cmd := newTestCommand(testData{
				Input:       strings.NewReader(""),
				Output:      &bytes.Buffer{},
				ErrorOutput: &bytes.Buffer{},
				UsageError:  func(err error) error { return err },
				Pooled: func(context.Context, pixivcmd.Request, func(context.Context, *pixiv.Client) (bool, error)) error {
					pooledCalls++
					return nil
				},
			})
			cmd.SetArgs(test.args)

			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), `type "all" is not supported by this command`) && !strings.Contains(err.Error(), `type "user" is not supported by this command`) {
				t.Fatalf("Execute error = %v, want unsupported type", err)
			}
			if pooledCalls != 0 {
				t.Fatalf("Pooled called %d time(s), want no network call", pooledCalls)
			}
		})
	}
}

func TestBookmarkAddConsumesNovelRecordsOnlyWithNovelType(t *testing.T) {
	transport := bookmarkRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodPost || request.URL.Path != "/v2/novel/bookmark/add" {
			t.Fatalf("request = %s %s, want POST /v2/novel/bookmark/add", request.Method, request.URL.Path)
		}
		if err := request.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		if request.PostForm.Get("novel_id") != "42" {
			t.Fatalf("form = %v, want novel_id 42", request.PostForm)
		}
		return bookmarkJSONResponse(request, http.StatusOK, `{}`), nil
	})
	client, err := pixiv.NewWith("test-access-token", pixiv.Options{HTTPClient: &http.Client{Transport: transport}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}

	cmd := newTestCommand(testData{
		Input:       strings.NewReader(`{"id":"42","type":"novel","url":"https://www.pixiv.net/novel/42"}` + "\n"),
		Output:      &bytes.Buffer{},
		ErrorOutput: &bytes.Buffer{},
		UsageError:  func(err error) error { return err },
		Pooled: func(ctx context.Context, _ pixivcmd.Request, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
			_, err := attempt(ctx, client)
			return err
		},
	})
	cmd.SetArgs([]string{"add", "--type", "novel"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
}

func TestBookmarkAddRejectsArtworkRecordWithNovelType(t *testing.T) {
	var diagnostics bytes.Buffer
	cmd := newTestCommand(testData{
		Input:       strings.NewReader(`{"id":"42","type":"artwork","url":"https://www.pixiv.net/artworks/42"}` + "\n"),
		Output:      &bytes.Buffer{},
		ErrorOutput: &diagnostics,
		UsageError:  func(err error) error { return err },
		Pooled: func(context.Context, pixivcmd.Request, func(context.Context, *pixiv.Client) (bool, error)) error {
			t.Fatal("Pooled must not be called for a namespace-mismatched record")
			return nil
		},
	})
	cmd.SetArgs([]string{"add", "--type", "novel", "--on-error", "fail-fast"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(diagnostics.String(), `"code":"unsupported_type"`) {
		t.Fatalf("Execute error = %v, diagnostics = %q, want unsupported_type", err, diagnostics.String())
	}
}

type testData struct {
	Input       io.Reader
	Output      io.Writer
	ErrorOutput io.Writer
	UsageError  func(error) error
	JSONOut     func(*bool) (bool, error)
	Pooled      func(context.Context, pixivcmd.Request, func(context.Context, *pixiv.Client) (bool, error)) error
}

func newTestCommand(d testData) *cobra.Command {
	env := commands.Env{In: d.Input, Out: d.Output, Err: d.ErrorOutput, WrapUsage: d.UsageError, ResolveJSON: d.JSONOut}
	return New(env, pixivcmd.Executor(d.Pooled))
}

func TestArtworkBookmarkRecordTypesConsumeArtworkByID(t *testing.T) {
	for _, operation := range []string{"bookmark_add", "bookmark_remove"} {
		t.Run(operation, func(t *testing.T) {
			var gotID int64
			err := input.ConsumeActionRecords(
				context.Background(),
				strings.NewReader(`{"id":"42","type":"artwork","url":"https://www.pixiv.net/artworks/42"}`+"\n"),
				&bytes.Buffer{}, operation, "fail-fast", visualRecordTypes,
				func(_ context.Context, id int64) error { gotID = id; return nil },
				func(err error) error { return err },
			)
			require.NoError(t, err)
			assert.Equal(t, int64(42), gotID)
		})
	}
}

func TestArtworkBookmarkRecordTypesStillRejectNonArtworkEntities(t *testing.T) {
	for _, recordType := range []string{"user", "novel"} {
		t.Run(recordType, func(t *testing.T) {
			called := false
			var diagnostics bytes.Buffer
			err := input.ConsumeActionRecords(
				context.Background(),
				strings.NewReader(fmt.Sprintf(`{"id":"42","type":%q,"url":"https://www.pixiv.net/example/42"}`+"\n", recordType)),
				&diagnostics, "bookmark_add", "fail-fast", visualRecordTypes,
				func(context.Context, int64) error { called = true; return nil },
				func(err error) error { return err },
			)
			require.Error(t, err)
			assert.Contains(t, diagnostics.String(), `"code":"unsupported_type"`)
			assert.False(t, called)
		})
	}
}
