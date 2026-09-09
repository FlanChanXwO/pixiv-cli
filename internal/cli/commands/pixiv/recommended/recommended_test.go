package recommended

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/spf13/cobra"
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

func TestCommandRecommendedContentTypeFiltersAcrossPages(t *testing.T) {
	for _, test := range []struct {
		name        string
		contentType string
		wantType    string
		wantIDs     []string
	}{
		{name: "illust", contentType: "illust", wantType: "illustration", wantIDs: []string{"7301", "7303"}},
		{name: "manga", contentType: "manga", wantType: "manga", wantIDs: []string{"7302", "7304"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			output := &bytes.Buffer{}
			var offsets []string
			transport := recommendedRoundTripFunc(func(request *http.Request) (*http.Response, error) {
				if request.URL.Path != "/v1/illust/recommended" {
					return nil, io.ErrUnexpectedEOF
				}
				if _, present := request.URL.Query()["content_type"]; present {
					t.Fatalf("recommended artwork request must not send content_type: %v", request.URL.Query())
				}
				offset, present := request.URL.Query()["offset"]
				if !present {
					offsets = append(offsets, "")
				} else {
					offsets = append(offsets, offset[0])
				}
				switch request.URL.Query().Get("offset") {
				case "":
					return recommendedJSONResponse(request, `{"illusts":[`+
						`{"id":7301,"title":"illustration one","type":"illust","create_date":"2026-01-05T00:00:00Z","user":{"id":21,"name":"artist"}},`+
						`{"id":7302,"title":"manga one","type":"manga","create_date":"2026-01-06T00:00:00Z","user":{"id":22,"name":"mangaka"}}`+
						`],"next_url":"https://app-api.pixiv.net/v1/illust/recommended?offset=0"}`), nil
				case "0":
					return recommendedJSONResponse(request, `{"illusts":[`+
						`{"id":7303,"title":"illustration two","type":"illust","create_date":"2026-01-07T00:00:00Z","user":{"id":23,"name":"artist two"}},`+
						`{"id":7304,"title":"manga two","type":"manga","create_date":"2026-01-08T00:00:00Z","user":{"id":24,"name":"mangaka two"}}`+
						`],"next_url":null}`), nil
				default:
					return nil, io.ErrUnexpectedEOF
				}
			})
			cmd := recommendedTestCommand(output, recommendedTestClient(t, transport))
			cmd.SetArgs([]string{"artwork", "--content-type", test.contentType, "--limit", "2", "--ndjson"})

			if err := cmd.Execute(); err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if got, want := offsets, []string{"", "0"}; !equalStrings(got, want) {
				t.Fatalf("recommended offsets = %v, want %v", got, want)
			}
			lines := strings.Split(strings.TrimSpace(output.String()), "\n")
			if len(lines) != len(test.wantIDs) {
				t.Fatalf("NDJSON lines = %d, want %d; output=%q", len(lines), len(test.wantIDs), output.String())
			}
			for index, line := range lines {
				var record map[string]any
				if err := json.Unmarshal([]byte(line), &record); err != nil {
					t.Fatalf("decode record %d: %v; output=%q", index, err, output.String())
				}
				wantID := test.wantIDs[index]
				if record["id"] != wantID || record["type"] != test.wantType {
					t.Fatalf("record %d = %#v, want %s %s", index, record, test.wantType, wantID)
				}
			}
		})
	}
}

func TestCommandRecommendedNovelAndUserContinueAcrossPages(t *testing.T) {
	for _, test := range []struct {
		name     string
		kind     string
		path     string
		wantIDs  []string
		response func(*http.Request, string) (*http.Response, error)
	}{
		{
			name:    "novel",
			kind:    "novel",
			path:    "/v1/novel/recommended",
			wantIDs: []string{"7501", "7502"},
			response: func(request *http.Request, offset string) (*http.Response, error) {
				if offset == "" {
					return recommendedJSONResponse(request, `{"novels":[{"id":7501,"title":"novel one","create_date":"2026-03-01T00:00:00Z","user":{"id":41,"name":"writer one"}}],"next_url":"https://app-api.pixiv.net/v1/novel/recommended?offset=0"}`), nil
				}
				if offset == "0" {
					return recommendedJSONResponse(request, `{"novels":[{"id":7502,"title":"novel two","create_date":"2026-03-02T00:00:00Z","user":{"id":42,"name":"writer two"}}],"next_url":null}`), nil
				}
				return nil, io.ErrUnexpectedEOF
			},
		},
		{
			name:    "user",
			kind:    "user",
			path:    "/v1/user/recommended",
			wantIDs: []string{"7601", "7602"},
			response: func(request *http.Request, offset string) (*http.Response, error) {
				if offset == "" {
					return recommendedJSONResponse(request, `{"user_previews":[{"user":{"id":7601,"name":"user one"}}],"next_url":"https://app-api.pixiv.net/v1/user/recommended?offset=0"}`), nil
				}
				if offset == "0" {
					return recommendedJSONResponse(request, `{"user_previews":[{"user":{"id":7602,"name":"user two"}}],"next_url":null}`), nil
				}
				return nil, io.ErrUnexpectedEOF
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			output := &bytes.Buffer{}
			var offsets []string
			transport := recommendedRoundTripFunc(func(request *http.Request) (*http.Response, error) {
				if request.URL.Path != test.path {
					return nil, io.ErrUnexpectedEOF
				}
				offset, present := request.URL.Query()["offset"]
				if !present {
					offsets = append(offsets, "")
				} else {
					offsets = append(offsets, offset[0])
				}
				return test.response(request, request.URL.Query().Get("offset"))
			})
			cmd := recommendedTestCommand(output, recommendedTestClient(t, transport))
			cmd.SetArgs([]string{test.kind, "--limit", "2", "--ndjson"})

			if err := cmd.Execute(); err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if got, want := offsets, []string{"", "0"}; !equalStrings(got, want) {
				t.Fatalf("recommended %s offsets = %v, want %v", test.kind, got, want)
			}
			lines := strings.Split(strings.TrimSpace(output.String()), "\n")
			if len(lines) != len(test.wantIDs) {
				t.Fatalf("NDJSON lines = %d, want %d; output=%q", len(lines), len(test.wantIDs), output.String())
			}
			for index, line := range lines {
				var record map[string]any
				if err := json.Unmarshal([]byte(line), &record); err != nil {
					t.Fatalf("decode record %d: %v; output=%q", index, err, output.String())
				}
				if record["id"] != test.wantIDs[index] || record["type"] != test.kind {
					t.Fatalf("record %d = %#v, want %s %s", index, record, test.kind, test.wantIDs[index])
				}
			}
		})
	}
}

func TestCommandRecommendedAllSeparatesArtworkSubtypesAndKeepsIndependentPages(t *testing.T) {
	output := &bytes.Buffer{}
	var paths []string
	transport := recommendedRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		paths = append(paths, request.URL.Path)
		switch request.URL.Path {
		case "/v1/illust/recommended":
			if request.URL.Query().Get("offset") == "" {
				return recommendedJSONResponse(request, `{"illusts":[`+
					`{"id":7401,"title":"illustration one","type":"illust","create_date":"2026-02-01T00:00:00Z","user":{"id":31,"name":"artist"}},`+
					`{"id":7402,"title":"manga one","type":"manga","create_date":"2026-02-02T00:00:00Z","user":{"id":32,"name":"mangaka"}}`+
					`],"next_url":"https://app-api.pixiv.net/v1/illust/recommended?offset=0"}`), nil
			}
			return recommendedJSONResponse(request, `{"illusts":[`+
				`{"id":7403,"title":"illustration two","type":"illust","create_date":"2026-02-03T00:00:00Z","user":{"id":33,"name":"artist two"}},`+
				`{"id":7404,"title":"manga two","type":"manga","create_date":"2026-02-04T00:00:00Z","user":{"id":34,"name":"mangaka two"}}`+
				`],"next_url":null}`), nil
		case "/v1/novel/recommended":
			return recommendedJSONResponse(request, `{"novels":[],"next_url":null}`), nil
		case "/v1/user/recommended":
			return recommendedJSONResponse(request, `{"user_previews":[],"next_url":null}`), nil
		default:
			return nil, io.ErrUnexpectedEOF
		}
	})
	client := recommendedTestClient(t, transport)
	cmd := recommendedTestCommand(output, client)
	cmd.SetArgs([]string{"--type", "all", "--limit", "2", "--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got, want := paths, []string{
		"/v1/illust/recommended", "/v1/illust/recommended",
		"/v1/illust/recommended", "/v1/illust/recommended",
		"/v1/novel/recommended", "/v1/user/recommended",
	}; !equalStrings(got, want) {
		t.Fatalf("recommended all paths = %v, want %v", got, want)
	}
	var envelope struct {
		Illusts []pixiv.ArtworkDTO     `json:"illusts"`
		Manga   []pixiv.ArtworkDTO     `json:"manga"`
		Novels  []pixiv.NovelDTO       `json:"novels"`
		Users   []pixiv.UserPreviewDTO `json:"user_previews"`
	}
	if err := json.Unmarshal(output.Bytes(), &envelope); err != nil {
		t.Fatalf("decode aggregate JSON: %v; output=%q", err, output.String())
	}
	if got, want := artworkIDs(envelope.Illusts), []int64{7401, 7403}; !equalInt64s(got, want) {
		t.Fatalf("illusts = %v, want %v", got, want)
	}
	if got, want := artworkIDs(envelope.Manga), []int64{7402, 7404}; !equalInt64s(got, want) {
		t.Fatalf("manga = %v, want %v", got, want)
	}
	if len(envelope.Novels) != 0 || len(envelope.Users) != 0 {
		t.Fatalf("empty aggregate streams = novels:%v users:%v", envelope.Novels, envelope.Users)
	}
}

func TestCommandRecommendedAllKeepsPositionalCompatibility(t *testing.T) {
	output := &bytes.Buffer{}
	transport := recommendedRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Path {
		case "/v1/illust/recommended":
			return recommendedJSONResponse(request, `{"illusts":[],"next_url":null}`), nil
		case "/v1/novel/recommended":
			return recommendedJSONResponse(request, `{"novels":[],"next_url":null}`), nil
		case "/v1/user/recommended":
			return recommendedJSONResponse(request, `{"user_previews":[],"next_url":null}`), nil
		default:
			return nil, io.ErrUnexpectedEOF
		}
	})
	cmd := recommendedTestCommand(output, recommendedTestClient(t, transport))
	cmd.SetArgs([]string{"all", "--limit", "1", "--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(output.Bytes(), &envelope); err != nil {
		t.Fatalf("decode positional all output: %v; output=%q", err, output.String())
	}
	for _, key := range []string{"illusts", "manga", "novels", "user_previews"} {
		if _, ok := envelope[key]; !ok {
			t.Fatalf("positional all output missing %q: %s", key, output.String())
		}
	}
}

type recommendedRoundTripFunc func(*http.Request) (*http.Response, error)

func (f recommendedRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func recommendedJSONResponse(request *http.Request, body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    request,
	}
}

func recommendedTestClient(t *testing.T, transport recommendedRoundTripFunc) *pixiv.Client {
	t.Helper()
	client, err := pixiv.NewWith("test-access-token", pixiv.Options{HTTPClient: &http.Client{Transport: transport}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}
	return client
}

func recommendedTestCommand(output io.Writer, client *pixiv.Client) *cobra.Command {
	return New(Dependencies{
		Input:      strings.NewReader(""),
		Output:     output,
		UsageError: func(err error) error { return err },
		JSONOut:    func(*bool) (bool, error) { return true, nil },
		Pooled: func(ctx context.Context, _ Request, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
			_, err := attempt(ctx, client)
			return err
		},
	})
}

func artworkIDs(items []pixiv.ArtworkDTO) []int64 {
	ids := make([]int64, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	return ids
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func equalInt64s(left, right []int64) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
