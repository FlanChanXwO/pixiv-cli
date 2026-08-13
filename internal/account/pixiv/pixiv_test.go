package pixiv_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/account/pixiv"
)

func TestAccountCredentialCopyAndSafeFormatting(t *testing.T) {
	input := []byte("refresh-secret")
	account := pixiv.New(42, "artist", input)
	input[0] = 'X'
	copyValue := account.RefreshTokenCopy()
	copyValue[0] = 'Y'
	if string(account.RefreshTokenCopy()) != "refresh-secret" {
		t.Fatal("credential was not defensively copied")
	}
	formatted := fmt.Sprintf("%+v", account)
	if strings.Contains(formatted, "refresh-secret") || strings.Contains(formatted, "refreshToken") {
		t.Fatalf("safe formatting leaked credential: %s", formatted)
	}
}
