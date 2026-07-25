package logging

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestLogOperationWritesStableSafeFields(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))
	LogOperation(logger, OperationEvent{
		Component:     "cli",
		Operation:     "search_illust",
		Backend:       "app_api",
		Duration:      25 * time.Millisecond,
		Result:        ResultError,
		ErrorCode:     "upstream_error",
		Status:        502,
		TransportKind: "timeout",
		IllustID:      42,
		UserID:        7,
	})

	var event map[string]any
	if err := json.Unmarshal(output.Bytes(), &event); err != nil {
		t.Fatalf("decode operation event: %v", err)
	}
	for key, want := range map[string]any{
		"msg":            "pixiv operation",
		"level":          "ERROR",
		"component":      "cli",
		"operation":      "search_illust",
		"backend":        "app_api",
		"result":         "error",
		"error_code":     "upstream_error",
		"status":         float64(502),
		"transport_kind": "timeout",
		"illust_id":      float64(42),
		"user_id":        float64(7),
	} {
		if event[key] != want {
			t.Fatalf("event[%q] = %#v, want %#v", key, event[key], want)
		}
	}
	if _, ok := event["duration"]; !ok {
		t.Fatal("operation event is missing duration")
	}
	source, ok := event["source"].(string)
	if !ok || !strings.HasPrefix(source, "internal/logging/logger_test.go:") {
		t.Fatalf("operation event source = %#v, want repository-relative logger_test.go location", event["source"])
	}
}

func TestLogOperationOmitsUnrecognizedTransportKind(t *testing.T) {
	t.Parallel()

	const hostile = "https://proxy-user:proxy-pass@host.invalid/?token=secret"
	var output bytes.Buffer
	LogOperation(slog.New(slog.NewJSONHandler(&output, nil)), OperationEvent{Result: ResultError, TransportKind: hostile})
	if strings.Contains(output.String(), hostile) || strings.Contains(output.String(), "transport_kind") {
		t.Fatalf("operation log accepted hostile transport kind: %s", output.String())
	}
}

func TestLogRateLimitRetryUsesOnlySafeRetryMetadata(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	LogRateLimitRetry(slog.New(slog.NewJSONHandler(&output, nil)), 3*time.Second, 2)
	var event map[string]any
	if err := json.Unmarshal(output.Bytes(), &event); err != nil {
		t.Fatalf("decode retry event: %v", err)
	}
	for key, want := range map[string]any{
		"msg":       "pixiv app api rate limit retry",
		"level":     "INFO",
		"component": "pixiv_app_api",
		"operation": "read",
		"result":    "rate_limit_retry",
		"status":    float64(429),
		"attempt":   float64(2),
	} {
		if event[key] != want {
			t.Fatalf("event[%q] = %#v, want %#v", key, event[key], want)
		}
	}
	if _, ok := event["retry_after"]; !ok {
		t.Fatal("rate-limit retry event is missing retry_after")
	}
	for _, forbidden := range []string{"url", "header", "token", "response", "body"} {
		if _, exists := event[forbidden]; exists {
			t.Fatalf("rate-limit retry event unexpectedly contains %q", forbidden)
		}
	}
}

func TestOrDiscardAcceptsNilLogger(t *testing.T) {
	t.Parallel()

	LogOperation(nil, OperationEvent{Result: ResultSuccess})
	LogRateLimitRetry(nil, time.Second, 2)
}
