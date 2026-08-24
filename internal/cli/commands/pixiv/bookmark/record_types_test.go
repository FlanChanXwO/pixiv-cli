package bookmark

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/cli/pipeline"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestArtworkBookmarkRecordTypesConsumeArtworkByID(t *testing.T) {
	assert.Equal(t, map[string]struct{}{
		"artwork": {},
		"illust":  {},
		"manga":   {},
		"ugoira":  {},
	}, visualRecordTypes)

	for _, operation := range []string{"bookmark_add", "bookmark_remove"} {
		t.Run(operation, func(t *testing.T) {
			var gotID int64
			err := pipeline.ConsumeActionRecords(
				context.Background(),
				strings.NewReader(`{"id":"42","type":"artwork","url":"https://www.pixiv.net/artworks/42"}`+"\n"),
				&bytes.Buffer{}, operation, "fail-fast", visualRecordTypes,
				func(_ context.Context, id int64) error { gotID = id; return nil },
				func(err error) error { return err },
			)
			require.NoError(t, err)
			assert.Equal(t, int64(42), gotID)
		})
	}
}

func TestArtworkBookmarkRecordTypesStillRejectNonArtworkEntities(t *testing.T) {
	for _, recordType := range []string{"user", "novel"} {
		t.Run(recordType, func(t *testing.T) {
			called := false
			var diagnostics bytes.Buffer
			err := pipeline.ConsumeActionRecords(
				context.Background(),
				strings.NewReader(fmt.Sprintf(`{"id":"42","type":%q,"url":"https://www.pixiv.net/example/42"}`+"\n", recordType)),
				&diagnostics, "bookmark_add", "fail-fast", visualRecordTypes,
				func(context.Context, int64) error { called = true; return nil },
				func(err error) error { return err },
			)
			require.Error(t, err)
			assert.Contains(t, diagnostics.String(), `"code":"unsupported_type"`)
			assert.False(t, called)
		})
	}
}
