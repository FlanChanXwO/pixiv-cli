package protocol_test

import (
	"encoding/json"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/protocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 本文件是 goal-1 t17 的契约层：RequiredList[T] / RequiredObject[T] 是从
// 26 + 3 处完全相同的 endpoint 局部实现收口而来的共享存在性解码类型。
//
// §5.3 要求必须保护的六种情形在这里逐条固定：字段缺失、字段为 null、合法空数组、
// 合法对象、字段类型错误、解码失败；另加"同一值被重新解码"。
//
// 语义要点（与收口前的局部实现逐字一致）：
//   - Present 在 UnmarshalJSON 被调用时即为 true——它表达的是"字段出现在 JSON 中"；
//   - Valid 表示"字段存在且能成功解码为 T"；
//   - null 属"存在但无效"（Present=true, Valid=false），不是缺失。

type requiredListPayload struct {
	Items protocol.RequiredList[string] `json:"items"`
}

type requiredObjectPayload struct {
	Value protocol.RequiredObject[string] `json:"value"`
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
		name        string
		body        string
		wantPresent bool
		wantValid   bool
		wantValue   string
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
			body:        `{"value":null}`,
			wantPresent: true,
			wantValid:   false,
		},
		{
			name:        "object is present and valid",
			body:        `{"value":"hello"}`,
			wantPresent: true,
			wantValid:   true,
			wantValue:   "hello",
		},
		{
			name:        "empty string is present and valid",
			body:        `{"value":""}`,
			wantPresent: true,
			wantValid:   true,
			wantValue:   "",
		},
		{
			name:        "wrong type is present but invalid",
			body:        `{"value":42}`,
			wantPresent: true,
			wantValid:   false,
			wantErr:     true,
		},
		{
			name:        "array where object expected fails to decode",
			body:        `{"value":[1]}`,
			wantPresent: true,
			wantValid:   false,
			wantErr:     true,
		},
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

// TestRequiredTypesResetOnRepeatedDecode 覆盖 §5.3 点名的"同一值被重新解码"：
// UnmarshalJSON 必须整体重置状态，不能让上一次的解码结果残留。
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

	t.Run("object resets stale state", func(t *testing.T) {
		var payload requiredObjectPayload
		require.NoError(t, json.Unmarshal([]byte(`{"value":"stale"}`), &payload))
		require.True(t, payload.Value.Valid)
		require.Equal(t, "stale", payload.Value.Value)

		// 第二次解码失败：不得保留上一次的值。
		require.Error(t, json.Unmarshal([]byte(`{"value":42}`), &payload))
		assert.True(t, payload.Value.Present)
		assert.False(t, payload.Value.Valid)
		assert.Empty(t, payload.Value.Value, "a failed re-decode must not keep the previous value")
	})

	t.Run("absent field after present resets", func(t *testing.T) {
		var payload requiredListPayload
		require.NoError(t, json.Unmarshal([]byte(`{"items":["x"]}`), &payload))
		require.True(t, payload.Items.Present)

		// 反序列化到新值时字段缺失：不得沿用旧值的 Presence。
		var fresh requiredListPayload
		require.NoError(t, json.Unmarshal([]byte(`{}`), &fresh))
		assert.False(t, fresh.Items.Present)
		assert.False(t, fresh.Items.Valid)
	})
}

// TestRequiredTypesCarryNonStringPayloads 确认收口后的泛型实现仍能承载各 endpoint
// 实际使用的元素/对象类型（这里用结构体与数字验证 T 不被特化到 string）。
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
