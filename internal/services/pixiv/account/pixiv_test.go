package pixiv_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	accountpixiv "github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/account"
)

func TestAccountCredentialCopyAndSafeFormatting(t *testing.T) {
	input := []byte("refresh-secret")
	account := accountpixiv.New(42, "artist", input)
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
	goFormatted := fmt.Sprintf("%#v", account)
	if strings.Contains(goFormatted, "refresh-secret") || strings.Contains(goFormatted, "refreshToken") {
		t.Fatalf("GoString formatting leaked credential: %s", goFormatted)
	}
}

func TestAccountWithTokenRequiresExplicitSecretAccessor(t *testing.T) {
	const secret = "export-secret"
	account := accountpixiv.NewAccountWithToken(42, "artist", true, secret)
	for _, formatted := range []string{
		fmt.Sprintf("%v", account),
		fmt.Sprintf("%+v", account),
		fmt.Sprintf("%#v", account),
	} {
		if strings.Contains(formatted, secret) || strings.Contains(formatted, "refreshToken") {
			t.Fatalf("AccountWithToken formatting leaked credential: %s", formatted)
		}
	}
	body, err := json.Marshal(account)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), secret) {
		t.Fatalf("AccountWithToken JSON leaked credential: %s", body)
	}
	if account.RefreshToken() != secret {
		t.Fatalf("RefreshToken() = %q, want %q", account.RefreshToken(), secret)
	}
}
