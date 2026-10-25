package pixivaccount

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/logging"
	"github.com/stretchr/testify/require"
)

func TestAccountFormattingKeepsCredentialsPrivate(t *testing.T) {
	const secret = "pixiv-refresh-canary"
	for _, account := range []any{New(42, "artist", []byte(secret)), newAccountWithToken(42, "artist", true, secret)} {
		var output bytes.Buffer
		for _, format := range []string{"%v", "%+v", "%#v"} {
			fmt.Fprintf(&output, format, account)
		}
		for _, format := range []string{logging.FormatText, logging.FormatJSON} {
			logger, err := logging.New(&output, "info", format)
			require.NoError(t, err)
			logger.Info("account", slog.Any("account", account))
		}
		require.Contains(t, output.String(), "artist")
		// %#v 的切片类型名随值所在位置变化，只检查字节内容。
		for _, leaked := range []string{secret, fmt.Sprint([]byte(secret)), strings.TrimPrefix(fmt.Sprintf("%#v", []byte(secret)), "[]byte"), base64.StdEncoding.EncodeToString([]byte(secret))} {
			require.NotContains(t, output.String(), leaked)
		}
	}
}
