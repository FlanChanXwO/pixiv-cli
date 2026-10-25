package logging_test

import (
	"bytes"
	"log/slog"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/logging"
	"github.com/stretchr/testify/require"
)

func TestTextHandlerResolvesPrivateLogValues(t *testing.T) {
	var output bytes.Buffer
	logger, err := logging.New(&output, "info", logging.FormatText)
	require.NoError(t, err)
	logger.Info("request", slog.Any("token", privateLogValue("credential-canary")))
	require.Contains(t, output.String(), "<redacted>")
	require.NotContains(t, output.String(), "credential-canary")
}

type privateLogValue string

func (privateLogValue) LogValue() slog.Value { return slog.StringValue("<redacted>") }
