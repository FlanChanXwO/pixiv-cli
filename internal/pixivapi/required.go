// Required response-field decoding shared by Pixiv endpoint mappings.

package pixivapi

import (
	"bytes"
	"encoding/json"
)

// Present records whether a response field appeared; Valid records successful
// decoding for T. Explicit null is present but invalid. Endpoint owners decide
// which fields are required and report their own contextual failures.
// A reused outer wire value must be cleared before decoding: missing fields
// do not invoke these decoders.

// RequiredList 解码一个必需数组字段，并保留其存在性与有效性。
type RequiredList[T any] struct {
	Items   []T
	Present bool
	Valid   bool
}

func (l *RequiredList[T]) UnmarshalJSON(data []byte) error {
	*l = RequiredList[T]{Present: true}
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return nil
	}
	if err := json.Unmarshal(data, &l.Items); err != nil {
		return err
	}
	l.Valid = true
	return nil
}

// RequiredObject 解码一个必需对象字段，并保留其存在性与有效性。
type RequiredObject[T any] struct {
	Value   T
	Present bool
	Valid   bool
}

func (o *RequiredObject[T]) UnmarshalJSON(data []byte) error {
	*o = RequiredObject[T]{Present: true}
	data = bytes.TrimSpace(data)
	if bytes.Equal(data, []byte("null")) {
		return nil
	}
	if err := json.Unmarshal(data, &o.Value); err != nil {
		return err
	}
	o.Valid = true
	return nil
}
