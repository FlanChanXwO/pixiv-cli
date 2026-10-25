package record_test

import (
	"encoding/json"
	"strconv"
	"testing"

	record "github.com/FlanChanXwO/pixiv-cli/internal/record"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewIdentityRecordBuildsCanonicalPixivIdentity(t *testing.T) {
	tests := []struct {
		name       string
		id         int64
		recordType string
		wantURL    string
		wantErr    bool
	}{
		{name: "artwork", id: 42, recordType: "artwork", wantURL: "https://www.pixiv.net/artworks/42"},
		{name: "user", id: 7, recordType: "user", wantURL: "https://www.pixiv.net/users/7"},
		{name: "zero id", id: 0, recordType: "artwork", wantErr: true},
		{name: "negative id", id: -1, recordType: "artwork", wantErr: true},
		{name: "empty type", id: 1, wantErr: true},
		{name: "artwork subtype", id: 1, recordType: "illust", wantErr: true},
		{name: "unknown type", id: 1, recordType: "novel", wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			identity, err := record.NewIdentityRecord(test.id, test.recordType)
			if test.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.recordType, identity.Type())
			assert.Equal(t, test.wantURL, identity.URL())
			id := strconv.FormatInt(test.id, 10)
			assert.Equal(t, id, identity.ID())

			body, err := json.Marshal(identity)
			require.NoError(t, err)
			assert.JSONEq(t, `{"id":"`+id+`","type":"`+test.recordType+`","url":"`+test.wantURL+`"}`, string(body))
		})
	}
}
