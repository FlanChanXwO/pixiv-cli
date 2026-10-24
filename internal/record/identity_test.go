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
	}{
		{name: "artwork", id: 42, recordType: "artwork", wantURL: "https://www.pixiv.net/artworks/42"},
		{name: "user", id: 7, recordType: "user", wantURL: "https://www.pixiv.net/users/7"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			identity, err := record.NewIdentityRecord(test.id, test.recordType)
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

func TestNewIdentityRecordRejectsInvalidIdentity(t *testing.T) {
	tests := []struct {
		name       string
		id         int64
		recordType string
	}{
		{name: "zero id", id: 0, recordType: "artwork"},
		{name: "negative id", id: -1, recordType: "artwork"},
		{name: "empty type", id: 1},
		{name: "artwork subtype", id: 1, recordType: "illust"},
		{name: "unknown type", id: 1, recordType: "novel"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := record.NewIdentityRecord(test.id, test.recordType)
			require.Error(t, err)
		})
	}
}
