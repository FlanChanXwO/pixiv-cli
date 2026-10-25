package pixivapi_test

import (
	"encoding/json"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixivapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type zipURLsDTO struct {
	Medium   string `json:"medium"`
	Original string `json:"original"`
}

// TestRequiredTypesPresenceAndReset 覆盖 RequiredList/RequiredObject 的
// present/valid 语义、重复解码清零与非字符串 payload。
func TestRequiredTypesPresenceAndReset(t *testing.T) {
	for _, test := range []struct {
		name        string
		body        string
		wantPresent bool
		wantValid   bool
		wantItems   []string
		wantErr     bool
	}{
		{name: "absent", body: `{}`},
		{name: "null", body: `{"items":null}`, wantPresent: true},
		{name: "empty", body: `{"items":[]}`, wantPresent: true, wantValid: true, wantItems: []string{}},
		{name: "populated", body: `{"items":["a","b"]}`, wantPresent: true, wantValid: true, wantItems: []string{"a", "b"}},
		{name: "wrong element", body: `{"items":[1]}`, wantPresent: true, wantErr: true},
		{name: "wrong container", body: `{"items":"x"}`, wantPresent: true, wantErr: true},
	} {
		t.Run("list/"+test.name, func(t *testing.T) {
			var payload struct {
				Items pixivapi.RequiredList[string] `json:"items"`
			}
			err := json.Unmarshal([]byte(test.body), &payload)
			if test.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, test.wantPresent, payload.Items.Present)
			assert.Equal(t, test.wantValid, payload.Items.Valid)
			if test.wantItems != nil {
				assert.Equal(t, test.wantItems, payload.Items.Items)
			}
		})
	}

	for _, test := range []struct {
		name                string
		body                string
		present, valid, err bool
	}{
		{name: "absent", body: `{}`},
		{name: "null", body: `{"value":null}`, present: true},
		{name: "empty object", body: `{"value":{}}`, present: true, valid: true},
		{name: "populated", body: `{"value":{"medium":"m","original":"o"}}`, present: true, valid: true},
		{name: "wrong field", body: `{"value":{"medium":42}}`, present: true, err: true},
		{name: "number container", body: `{"value":42}`, present: true, err: true},
		{name: "array container", body: `{"value":[]}`, present: true, err: true},
	} {
		t.Run("object/"+test.name, func(t *testing.T) {
			var payload struct {
				Value pixivapi.RequiredObject[zipURLsDTO] `json:"value"`
			}
			err := json.Unmarshal([]byte(test.body), &payload)
			if test.err {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, test.present, payload.Value.Present)
			assert.Equal(t, test.valid, payload.Value.Valid)
		})
	}

	var list struct {
		Items pixivapi.RequiredList[string] `json:"items"`
	}
	require.NoError(t, json.Unmarshal([]byte(`{"items":["first","second"]}`), &list))
	require.NoError(t, json.Unmarshal([]byte(`{"items":null}`), &list))
	assert.True(t, list.Items.Present)
	assert.False(t, list.Items.Valid)
	assert.Nil(t, list.Items.Items)

	var object struct {
		Value pixivapi.RequiredObject[zipURLsDTO] `json:"value"`
	}
	require.NoError(t, json.Unmarshal([]byte(`{"value":{"medium":"old","original":"stale"}}`), &object))
	require.NoError(t, json.Unmarshal([]byte(`{"value":{"medium":"new"}}`), &object))
	assert.True(t, object.Value.Present)
	assert.True(t, object.Value.Valid)
	assert.Equal(t, zipURLsDTO{Medium: "new"}, object.Value.Value)

	type nested struct {
		ID int `json:"id"`
	}
	var nonString struct {
		Items pixivapi.RequiredList[nested] `json:"items"`
	}
	require.NoError(t, json.Unmarshal([]byte(`{"items":[{"id":1},{"id":2}]}`), &nonString))
	assert.True(t, nonString.Items.Valid)
	assert.Equal(t, []nested{{ID: 1}, {ID: 2}}, nonString.Items.Items)
}
