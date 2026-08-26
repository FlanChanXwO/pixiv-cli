package record_test

import (
	"encoding/json"
	"strconv"
	"testing"

	record "github.com/FlanChanXwO/pixiv-cli/internal/shared/record"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewIdentityRecordBuildsCanonicalPixivIdentity(t *testing.T) {
	tests := []struct {
		name       string
		id         int64
		recordType string
		url        string
	}{
		{name: "artwork", id: 42, recordType: "artwork", url: "https://www.pixiv.net/artworks/42"},
		{name: "user", id: 7, recordType: "user", url: "https://www.pixiv.net/users/7"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			identity, err := record.NewIdentityRecord(test.id, test.recordType, test.url)
			require.NoError(t, err)
			assert.Equal(t, test.recordType, identity.Type())
			assert.Equal(t, test.url, identity.URL())
			id := strconv.FormatInt(test.id, 10)
			assert.Equal(t, id, identity.ID())

			body, err := json.Marshal(identity)
			require.NoError(t, err)
			assert.JSONEq(t, `{"id":"`+id+`","type":"`+test.recordType+`","url":"`+test.url+`"}`, string(body))
		})
	}
}

func TestNewIdentityRecordRejectsInvalidIdentity(t *testing.T) {
	tests := []struct {
		name       string
		id         int64
		recordType string
		url        string
	}{
		{name: "zero id", id: 0, recordType: "artwork", url: "https://www.pixiv.net/artworks/0"},
		{name: "negative id", id: -1, recordType: "artwork", url: "https://www.pixiv.net/artworks/-1"},
		{name: "empty type", id: 1, url: "https://www.pixiv.net/artworks/1"},
		{name: "artwork subtype", id: 1, recordType: "illust", url: "https://www.pixiv.net/artworks/1"},
		{name: "unknown type", id: 1, recordType: "novel", url: "https://www.pixiv.net/novel/show.php?id=1"},
		{name: "empty url", id: 1, recordType: "artwork"},
		{name: "wrong scheme", id: 1, recordType: "artwork", url: "http://www.pixiv.net/artworks/1"},
		{name: "wrong host", id: 1, recordType: "artwork", url: "https://pixiv.net/artworks/1"},
		{name: "wrong route", id: 1, recordType: "artwork", url: "https://www.pixiv.net/users/1"},
		{name: "wrong id", id: 1, recordType: "artwork", url: "https://www.pixiv.net/artworks/2"},
		{name: "query", id: 1, recordType: "user", url: "https://www.pixiv.net/users/1?private=value"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := record.NewIdentityRecord(test.id, test.recordType, test.url)
			require.Error(t, err)
		})
	}
}
