package browsercookies_test

import (
	"bytes"
	"fmt"
	"log/slog"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/browsercookies"
	"github.com/FlanChanXwO/pixiv-cli/internal/logging"
	"github.com/stretchr/testify/require"
)

func TestCookieFormattingKeepsSessionPrivate(t *testing.T) {
	const secret = "FANBOXSESSID=cookie-canary"
	cookie := browsercookies.NewSecret(secret)
	for _, format := range []string{"%v", "%+v", "%#v"} {
		require.NotContains(t, fmt.Sprintf(format, cookie), secret)
	}
	for _, format := range []string{logging.FormatText, logging.FormatJSON} {
		var output bytes.Buffer
		logger, err := logging.New(&output, "info", format)
		require.NoError(t, err)
		logger.Info("cookie", slog.Any("cookie", cookie))
		require.Contains(t, output.String(), "cookie")
		require.NotContains(t, output.String(), secret)
	}
}
