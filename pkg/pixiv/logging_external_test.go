package pixiv_test

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/pkg/pixiv"
)

func TestClientLoggerEmitsSafeStructuredFailureAndNilIsNoop(t *testing.T) {
	const accessSecret = "access-secret-canary"
	const querySecret = "query-secret-canary"
	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))
	client, err := pixiv.NewClient(pixiv.Options{
		AccessToken:   accessSecret,
		AppAPIBaseURL: "https://api.example.invalid/?q=" + querySecret,
		Logger:        logger,
	})
	if err != nil {
		t.Fatal(err)
	}
	err = client.AddBookmark(context.Background(), pixiv.AddBookmarkRequest{IllustID: -7})
	if err == nil {
		t.Fatal("AddBookmark unexpectedly succeeded")
	}
	got := output.String()
	for _, field := range []string{`"component":"pixiv_sdk"`, `"operation":"add_bookmark"`, `"backend":"local"`, `"result":"error"`, `"error_code":"invalid_argument"`, `"illust_id":-7`} {
		if !strings.Contains(got, field) {
			t.Fatalf("log missing %s: %s", field, got)
		}
	}
	if !strings.Contains(got, `"level":"ERROR"`) {
		t.Fatalf("failure was not logged at error level: %s", got)
	}
	for _, secret := range []string{accessSecret, querySecret} {
		if strings.Contains(got, secret) {
			t.Fatalf("log leaked secret %q: %s", secret, got)
		}
	}

	quiet, err := pixiv.NewClient(pixiv.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err := quiet.AddBookmark(context.Background(), pixiv.AddBookmarkRequest{IllustID: -1}); err == nil {
		t.Fatal("nil logger client unexpectedly succeeded")
	}
}

func TestOpenDefaultLogsOneEventForOnePublicOperation(t *testing.T) {
	var output bytes.Buffer
	client, err := pixiv.OpenDefault(pixiv.Options{Logger: slog.New(slog.NewJSONHandler(&output, nil))})
	if err != nil {
		t.Fatal(err)
	}
	if err := client.AddBookmark(context.Background(), pixiv.AddBookmarkRequest{IllustID: -1}); err == nil {
		t.Fatal("AddBookmark unexpectedly succeeded")
	}
	if got := strings.Count(output.String(), `"operation":"add_bookmark"`); got != 1 {
		t.Fatalf("add bookmark log count = %d: %s", got, output.String())
	}
}

func TestOpenDefaultCurrentUserSnapshotFailureLogsOnlyCurrentUserOperation(t *testing.T) {
	configPath := t.TempDir() + "/config.toml"
	if err := os.WriteFile(configPath, []byte("[logging]\nformat = 'invalid'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	client, err := pixiv.OpenDefault(pixiv.Options{ConfigFilePath: configPath, Logger: slog.New(slog.NewJSONHandler(&output, nil))})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.CurrentUserID(context.Background()); err == nil {
		t.Fatal("CurrentUserID unexpectedly succeeded")
	}
	got := output.String()
	if strings.Count(got, `"operation":"current_user_id"`) != 1 || strings.Contains(got, `"operation":"snapshot"`) {
		t.Fatalf("unexpected snapshot log sequence: %s", got)
	}
}

func TestClientLoggerRedactsRealUpstreamFailureCanaries(t *testing.T) {
	const token = "access-token-canary"
	const query = "query-canary"
	const body = "response-body-canary"
	const cookie = "cookie-canary"
	const oauthCode = "oauth-code-canary"
	const verifier = "pkce-verifier-canary"
	var output bytes.Buffer
	client, err := pixiv.NewClient(pixiv.Options{
		AccessToken:   token,
		AppAPIBaseURL: "https://api.example.invalid/?secret=" + query,
		Logger:        slog.New(slog.NewJSONHandler(&output, nil)),
		HTTPClient: &http.Client{Transport: loggingRoundTripper(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusInternalServerError, Body: io.NopCloser(strings.NewReader(body + cookie + oauthCode + verifier)), Header: make(http.Header)}, nil
		})},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.SearchIllust(context.Background(), pixiv.SearchIllustRequest{Word: "safe"}); err == nil {
		t.Fatal("SearchIllust unexpectedly succeeded")
	}
	got := output.String()
	if !strings.Contains(got, `"operation":"search_illust"`) || !strings.Contains(got, `"error_code":"upstream_error"`) {
		t.Fatalf("missing structured failure: %s", got)
	}
	for _, secret := range []string{token, query, body, cookie, oauthCode, verifier} {
		if strings.Contains(got, secret) {
			t.Fatalf("log leaked %q: %s", secret, got)
		}
	}
}

type loggingRoundTripper func(*http.Request) (*http.Response, error)

func (f loggingRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}
