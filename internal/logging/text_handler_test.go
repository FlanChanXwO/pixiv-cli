package logging

import (
	"bytes"
	"context"
	"log/slog"
	"regexp"
	"testing"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/common/constants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTextHandlerCompactsOperationEvent(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	handler := NewTextHandler(&output, &slog.HandlerOptions{Level: slog.LevelDebug})
	record := slog.NewRecord(time.Date(2026, time.July, 23, 23, 55, 14, 168_000_000, time.FixedZone("CST", 8*60*60)), slog.LevelError, constants.OperationLogMessage, 0)
	record.AddAttrs(
		slog.String(constants.LogFieldComponent, "cli"),
		slog.String(constants.LogFieldOperation, "pixiv"),
		slog.String(constants.LogFieldBackend, constants.LogBackendLocal),
		slog.Duration(constants.LogFieldDuration, 16500*time.Nanosecond),
		slog.String(constants.LogFieldResult, ResultError),
		slog.String(constants.LogFieldErrorCode, ""),
		slog.Int(constants.LogFieldStatus, 0),
	)

	require.NoError(t, handler.Handle(context.Background(), record))
	line := output.String()
	assert.Regexp(t, regexp.MustCompile(`^2026-07-23 23:55:14\.168  ERROR \d+ --- \[cli\] pixiv +: pixiv error duration=16\.5µs\n$`), line)
	assert.NotContains(t, line, "msg=")
	assert.NotContains(t, line, "component=")
	assert.NotContains(t, line, "operation=")
	assert.NotContains(t, line, "backend=local")
	assert.NotContains(t, line, "error_code=")
	assert.NotContains(t, line, "status=0")
}

func TestTextHandlerIncludesOnlyUsefulOperationMetadata(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	handler := NewTextHandler(&output, nil)
	record := slog.NewRecord(time.Date(2026, time.July, 23, 23, 55, 14, 0, time.UTC), slog.LevelError, constants.OperationLogMessage, 0)
	record.AddAttrs(
		slog.String(constants.LogFieldComponent, "pixiv_sdk"),
		slog.String(constants.LogFieldOperation, "search_illust"),
		slog.String(constants.LogFieldBackend, "app_api"),
		slog.Duration(constants.LogFieldDuration, 25*time.Millisecond),
		slog.String(constants.LogFieldResult, ResultError),
		slog.String(constants.LogFieldErrorCode, "upstream_error"),
		slog.Int(constants.LogFieldStatus, 502),
		slog.String(constants.LogFieldTransportKind, "timeout"),
		slog.Int64(constants.LogFieldIllustID, 42),
		slog.Int64(constants.LogFieldUserID, 7),
	)

	require.NoError(t, handler.Handle(context.Background(), record))
	assert.Regexp(t, regexp.MustCompile(`^2026-07-23 23:55:14\.000  ERROR \d+ --- \[pixiv_sdk\] search_illust +: search_illust error backend=app_api error=upstream_error status=502 transport=timeout illust_id=42 user_id=7 duration=25ms\n$`), output.String())
}

func TestTextHandlerUsesOperationCallsiteAsSource(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	LogOperation(slog.New(NewTextHandler(&output, nil)), OperationEvent{
		Component: "cli",
		Operation: "pixiv",
		Result:    ResultError,
	})

	assert.Regexp(t, regexp.MustCompile(`^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}\.\d{3}  ERROR \d+ --- \[cli\] internal/logging/text_handler_test\.go:\d+ +: pixiv error\n$`), output.String())
	assert.NotContains(t, output.String(), "source=")
}
