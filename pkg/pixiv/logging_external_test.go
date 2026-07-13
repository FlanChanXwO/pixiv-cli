package pixiv_test

import (
	"bytes"
	"context"
	"log/slog"
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
