package fanbox_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/account/fanbox"
)

func TestAccountSessionCopyAndSafeFormatting(t *testing.T) {
	input := []byte("fanbox-session-secret")
	account := fanbox.New(7, "creator", "cid", input)
	input[0] = 'X'
	copyValue := account.SessionIDCopy()
	copyValue[0] = 'Y'
	if string(account.SessionIDCopy()) != "fanbox-session-secret" {
		t.Fatal("session was not defensively copied")
	}
	formatted := fmt.Sprintf("%+v", account)
	if strings.Contains(formatted, "fanbox-session-secret") || strings.Contains(formatted, "sessionID") {
		t.Fatalf("safe formatting leaked session: %s", formatted)
	}
}
