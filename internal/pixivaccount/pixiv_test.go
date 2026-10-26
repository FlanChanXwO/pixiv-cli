package pixivaccount

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/credential"
	"github.com/FlanChanXwO/pixiv-cli/internal/fanboxaccount"
	"github.com/FlanChanXwO/pixiv-cli/internal/logging"
	"github.com/stretchr/testify/require"
)

func TestAccountFormattingKeepsCredentialsPrivate(t *testing.T) {
	const secret = "credential-canary"
	cookie := credential.New([]byte(secret))
	account := New(42, "artist", []byte(secret))
	for _, value := range []any{account, &account, newAccountWithToken(42, "artist", true, secret), fanboxaccount.New(42, "artist", "creator", []byte(secret)), cookie} {
		var output bytes.Buffer
		for _, format := range []string{"%v", "%+v", "%#v"} {
			fmt.Fprintf(&output, format, value)
		}
		for _, handler := range []slog.Handler{slog.NewTextHandler(&output, nil), slog.NewJSONHandler(&output, nil)} {
			slog.New(handler).Info("credential", slog.Any("value", value))
		}
		for _, format := range []string{logging.FormatText, logging.FormatJSON} {
			logger, err := logging.New(&output, "info", format)
			require.NoError(t, err)
			logger.Info("credential", slog.Any("value", value))
		}
		if _, isSecret := value.(credential.Secret); !isSecret {
			require.Contains(t, output.String(), "artist")
		}
		// %#v 的切片类型名随值所在位置变化，只检查字节内容。
		for _, leaked := range []string{secret, fmt.Sprint([]byte(secret)), strings.TrimPrefix(fmt.Sprintf("%#v", []byte(secret)), "[]byte"), base64.StdEncoding.EncodeToString([]byte(secret))} {
			require.NotContains(t, output.String(), leaked)
		}
	}
	require.Equal(t, slog.StringValue("<redacted>"), slog.Any("cookie", cookie).Value.Resolve())
}
