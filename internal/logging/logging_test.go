package logging_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/logging"
)

func TestFromWithoutCallIsSilent(t *testing.T) {
	var output bytes.Buffer
	if logging.From(context.Background()) == nil {
		t.Fatal("From returned nil logger")
	}
	logging.From(context.Background()).Info("ignored")
	if output.Len() != 0 {
		t.Fatalf("background logger wrote %q", output.String())
	}
}

func TestTextHandlerAddsCallInfoAndCauseTree(t *testing.T) {
	var output bytes.Buffer
	logger, err := logging.New(&output, "debug", "text")
	if err != nil {
		t.Fatal(err)
	}
	ctx := logging.WithCall(context.Background(), logger, "pixiv download", "call-7")
	logger = logging.From(ctx)
	logger.ErrorContext(ctx, "Command failed", slog.Any(logging.AttrError, errors.Join(errors.New("first"), errors.New("second"))))

	text := output.String()
	for _, want := range []string{"Command failed", "operation=pixiv download", "call_id=call-7", "caused by: first", "caused by: second"} {
		if !strings.Contains(text, want) {
			t.Fatalf("text output missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "third") {
		t.Fatalf("private unwrap cause leaked: %s", text)
	}
}

func TestJSONHandlerUsesSharedAttributeKeys(t *testing.T) {
	var output bytes.Buffer
	logger, err := logging.New(&output, "warn", "json")
	if err != nil {
		t.Fatal(err)
	}
	ctx := logging.WithCall(context.Background(), logger, "pixiv_search_illust", "3")
	logger.WarnContext(ctx, "Request rejected by upstream",
		slog.String(logging.AttrMethod, "GET"),
		slog.String(logging.AttrHost, "app-api.pixiv.net"),
		slog.String(logging.AttrPath, "/v1/search/illust"),
		slog.Int(logging.AttrStatus, 429),
		slog.Duration(logging.AttrWait, 0),
		slog.Int(logging.AttrAttempt, 1),
		slog.String(logging.AttrSDKOperation, "SearchArtworks"),
		slog.Any(logging.AttrError, errors.New("rate limited")),
	)
	line := strings.TrimSpace(output.String())
	for _, want := range []string{`"operation":"pixiv_search_illust"`, `"call_id":"3"`, `"status":429`, `"sdk_operation":"SearchArtworks"`, `"error":"rate limited"`} {
		if !strings.Contains(line, want) {
			t.Fatalf("json output missing %s: %s", want, line)
		}
	}
}
