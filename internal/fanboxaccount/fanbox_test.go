package fanboxaccount_test

import (
	"bytes"
	"fmt"
	"log/slog"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/fanboxaccount"
	"github.com/FlanChanXwO/pixiv-cli/internal/logging"
	"github.com/stretchr/testify/require"
)

func TestAccountFormattingKeepsSessionPrivate(t *testing.T) {
	const secret = "fanbox-session-canary"
	account := fanboxaccount.New(42, "artist", "creator", []byte(secret))
	for _, format := range []string{"%v", "%+v", "%#v"} {
		require.NotContains(t, fmt.Sprintf(format, account), secret)
	}
	for _, format := range []string{logging.FormatText, logging.FormatJSON} {
		var output bytes.Buffer
		logger, err := logging.New(&output, "info", format)
		require.NoError(t, err)
		logger.Info("account", slog.Any("account", account))
		require.Contains(t, output.String(), "artist")
		require.NotContains(t, output.String(), secret)
	}
}
