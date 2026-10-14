package protocol_test

import (
	"encoding/json"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/protocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 通过共享类型的 JSON 边界验证存在性；对象 fixture 与 ugoiraZipURLsDTO 同形。
// 字段缺失不会调用字段 decoder，复用外层 DTO 前的清零由调用方负责。

type requiredListPayload struct {
	Items protocol.RequiredList[string] `json:"items"`
}

type zipURLsDTO struct {
	Medium   string `json:"medium"`
	Original string `json:"original"`
}

type requiredObjectPayload struct {
	Value protocol.RequiredObject[zipURLsDTO] `json:"value"`
}

func TestRequiredListUnmarshalContract(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		wantPresent bool
		wantValid   bool
		wantItems   []string
		wantErr     bool
	}{
		{
			name:        "field absent",
			body:        `{}`,
			wantPresent: false,
			wantValid:   false,
		},
		{
			name:        "field is null",
			body:        `{"items":null}`,
			wantPresent: true,
			wantValid:   false,
		},
		{
			name:        "empty array is present and valid",
			body:        `{"items":[]}`,
			wantPresent: true,
			wantValid:   true,
			wantItems:   []string{},
		},
		{
			name:        "populated array is present and valid",
			body:        `{"items":["a","b"]}`,
			wantPresent: true,
			wantValid:   true,
			wantItems:   []string{"a", "b"},
		},
		{
			name:        "wrong element type is present but invalid",
			body:        `{"items":[1,2]}`,
			wantPresent: true,
			wantValid:   false,
			wantErr:     true,
		},
		{
			name:        "wrong container type fails to decode",
			body:        `{"items":"not-an-array"}`,
			wantPresent: true,
			wantValid:   false,
			wantErr:     true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var payload requiredListPayload
			err := json.Unmarshal([]byte(test.body), &payload)
			if test.wantErr {
				require.Error(t, err, "decode failure must surface so the endpoint can reject the response")
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, test.wantPresent, payload.Items.Present, "Present must report field presence")
			assert.Equal(t, test.wantValid, payload.Items.Valid, "Valid must report presence plus successful decode")
			if test.wantItems != nil {
				assert.Equal(t, test.wantItems, payload.Items.Items)
			}
		})
	}
}

func TestRequiredObjectUnmarshalContract(t *testing.T) {
	tests := []struct {
		name, body                      string
		wantPresent, wantValid, wantErr bool
		wantValue                       zipURLsDTO
	}{
		{name: "field absent", body: `{}`},
		{name: "field is null", body: `{"value":null}`, wantPresent: true},
		{name: "empty object", body: `{"value":{}}`, wantPresent: true, wantValid: true},
		{name: "populated object", body: `{"value":{"medium":"m","original":"o"}}`, wantPresent: true, wantValid: true, wantValue: zipURLsDTO{Medium: "m", Original: "o"}},
		{name: "wrong field type", body: `{"value":{"medium":42}}`, wantPresent: true, wantErr: true},
		{name: "number container", body: `{"value":42}`, wantPresent: true, wantErr: true},
		{name: "string container", body: `{"value":"hello"}`, wantPresent: true, wantErr: true},
		{name: "boolean container", body: `{"value":true}`, wantPresent: true, wantErr: true},
		{name: "array container", body: `{"value":[]}`, wantPresent: true, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var payload requiredObjectPayload
			err := json.Unmarshal([]byte(test.body), &payload)
			if test.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, test.wantPresent, payload.Value.Present)
			assert.Equal(t, test.wantValid, payload.Value.Valid)
			if test.wantValid {
				assert.Equal(t, test.wantValue, payload.Value.Value)
			}
		})
	}
}

// TestRequiredTypesResetOnRepeatedDecode 覆盖同一值被重新解码：
// 只有实际被调用的字段 decoder 才能清除旧状态。
func TestRequiredTypesResetOnRepeatedDecode(t *testing.T) {
	t.Run("list resets stale state", func(t *testing.T) {
		var payload requiredListPayload
		require.NoError(t, json.Unmarshal([]byte(`{"items":["first","second"]}`), &payload))
		require.Equal(t, []string{"first", "second"}, payload.Items.Items)
		require.True(t, payload.Items.Valid)

		// 第二次解码为 null：Items 必须被清空，Valid 必须回退为 false。
		require.NoError(t, json.Unmarshal([]byte(`{"items":null}`), &payload))
		assert.True(t, payload.Items.Present)
		assert.False(t, payload.Items.Valid, "a null re-decode must clear validity")
		assert.Nil(t, payload.Items.Items, "a null re-decode must not keep the previous items")
	})

	t.Run("object resets on present fields", func(t *testing.T) {
		var payload requiredObjectPayload
		require.NoError(t, json.Unmarshal([]byte(`{"value":{"medium":"old","original":"stale"}}`), &payload))
		require.NoError(t, json.Unmarshal([]byte(`{"value":{"medium":"new"}}`), &payload))
		assert.Equal(t, zipURLsDTO{Medium: "new"}, payload.Value.Value)
		require.Error(t, json.Unmarshal([]byte(`{"value":{"medium":"partial","original":42}}`), &payload))
		assert.True(t, payload.Value.Present)
		assert.False(t, payload.Value.Valid)
		// 失败允许保留本次的部分解码数据，但不能带回上一次的字段。
		assert.Equal(t, zipURLsDTO{Medium: "partial"}, payload.Value.Value)
		require.NoError(t, json.Unmarshal([]byte(`{"value":null}`), &payload))
		assert.True(t, payload.Value.Present)
		assert.False(t, payload.Value.Valid)
		assert.Equal(t, zipURLsDTO{}, payload.Value.Value)
	})

	t.Run("caller resets reused outer DTO before missing fields", func(t *testing.T) {
		type response struct {
			Items protocol.RequiredList[string]       `json:"items"`
			Value protocol.RequiredObject[zipURLsDTO] `json:"value"`
		}
		var payload response
		require.NoError(t, json.Unmarshal([]byte(`{"items":["x"],"value":{"medium":"old"}}`), &payload))
		require.NoError(t, json.Unmarshal([]byte(`{}`), &payload))
		assert.True(t, payload.Items.Present)
		assert.True(t, payload.Value.Present)
		assert.Equal(t, []string{"x"}, payload.Items.Items)
		assert.Equal(t, "old", payload.Value.Value.Medium)
		payload = response{}
		require.NoError(t, json.Unmarshal([]byte(`{}`), &payload))
		assert.False(t, payload.Items.Present)
		assert.False(t, payload.Items.Valid)
		assert.False(t, payload.Value.Present)
		assert.False(t, payload.Value.Valid)
	})
}

// 非 struct 实例仅记录泛型的现有行为，不作为生产对象 DTO 的兼容证据。
func TestRequiredTypesCarryNonStringPayloads(t *testing.T) {
	type nested struct {
		ID int `json:"id"`
	}
	var list struct {
		Items protocol.RequiredList[nested] `json:"items"`
	}
	require.NoError(t, json.Unmarshal([]byte(`{"items":[{"id":1},{"id":2}]}`), &list))
	require.True(t, list.Items.Valid)
	assert.Equal(t, []nested{{ID: 1}, {ID: 2}}, list.Items.Items)

	var obj struct {
		Value protocol.RequiredObject[int] `json:"value"`
	}
	require.NoError(t, json.Unmarshal([]byte(`{"value":7}`), &obj))
	require.True(t, obj.Value.Valid)
	assert.Equal(t, 7, obj.Value.Value)
}
