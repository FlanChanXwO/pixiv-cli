package sdk_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

func TestResourceDTOIsOpaque(t *testing.T) {
	ref, err := sdk.NewResourceRef("pixiv", []byte("artwork:42:original"))
	if err != nil {
		t.Fatalf("NewResourceRef: %v", err)
	}
	expires := time.Now().Add(time.Hour)
	resource := sdk.Resource{
		Ref:                 ref,
		URL:                 "https://signed.example/secret?token=do-not-leak",
		RequestHeaders:      map[string]string{"Cookie": "secret-cookie"},
		ExpiresAt:           &expires,
		RequiresCredentials: true,
	}

	dto := sdk.ToResourceDTO(resource)
	if dto == nil {
		t.Fatal("resource with a reference must produce a DTO")
	}
	if dto.Ref != ref.String() {
		t.Fatalf("dto.Ref = %q, want %q", dto.Ref, ref.String())
	}
	if !dto.RequiresCredentials {
		t.Fatal("DTO lost RequiresCredentials")
	}

	data, err := json.Marshal(dto)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	encoded := string(data)
	for _, forbidden := range []string{"signed.example", "do-not-leak", "Cookie", "secret-cookie", "expires_at"} {
		if strings.Contains(encoded, forbidden) {
			t.Fatalf("resource DTO leaked %q: %s", forbidden, encoded)
		}
	}
	if !strings.Contains(encoded, `"ref":"`+ref.String()+`"`) {
		t.Fatalf("resource DTO omitted opaque ref: %s", encoded)
	}

	if dto := sdk.ToResourceDTO(sdk.Resource{}); dto != nil {
		t.Fatalf("zero resource produced DTO: %+v", dto)
	}
}
