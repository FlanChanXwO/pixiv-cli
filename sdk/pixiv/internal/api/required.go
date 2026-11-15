package api

import (
	"bytes"
	"encoding/json"
)

// RequiredList tracks presence (including null) separately from successful decoding.
// Reset an enclosing wire value before reuse because missing fields skip UnmarshalJSON.
type RequiredList[T any] struct {
	Items   []T
	Present bool
	Valid   bool
}

// UnmarshalJSON marks the field present before decoding, including on failure.
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

// RequiredObject tracks presence (including null) separately from successful decoding.
// Reset an enclosing wire value before reuse because missing fields skip UnmarshalJSON.
type RequiredObject[T any] struct {
	Value   T
	Present bool
	Valid   bool
}

// UnmarshalJSON marks the field present before decoding, including on failure.
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
