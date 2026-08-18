package record_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	recordpkg "github.com/FlanChanXwO/pixiv-cli/internal/shared/record"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/stretchr/testify/require"
)

func TestSafeRecordSerializationKeepsResourceRefAndOmitsTransportFields(t *testing.T) {
	ref, err := sdk.NewResourceRef("pixiv", []byte(`{"k":"artwork","id":42}`))
	require.NoError(t, err)
	expiresAt := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
	artwork := pixiv.Artwork{
		ID:   42,
		Kind: pixiv.ArtworkKindIllustration,
		Cover: pixiv.ImageResource{Resource: sdk.Resource{
			Ref:            ref,
			URL:            "https://signed.example/image.jpg?signature=secret",
			RequestHeaders: map[string]string{"Cookie": "session-secret"},
			ExpiresAt:      &expiresAt,
		}},
	}

	record, err := recordpkg.RecordFromArtworkDTO(pixiv.ToArtworkDTO(artwork))
	require.NoError(t, err)
	body, err := json.Marshal(record)
	require.NoError(t, err)
	text := string(body)
	require.Contains(t, text, ref.String())
	for _, secret := range []string{"signed.example", "signature=secret", "session-secret", "request_headers", "expires_at"} {
		if strings.Contains(text, secret) {
			t.Fatalf("safe record contains %q: %s", secret, text)
		}
	}
}
