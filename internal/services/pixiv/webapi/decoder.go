package webapi

import (
	"bytes"
	"encoding/json"
	"strconv"
)

func (e *ajaxEnvelope[T]) UnmarshalJSON(data []byte) error {
	var wire struct {
		Error   bool            `json:"error"`
		Message string          `json:"message"`
		Body    json.RawMessage `json:"body"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	e.Error = wire.Error
	e.Message = wire.Message
	body := bytes.TrimSpace(wire.Body)
	if len(body) == 0 || bytes.Equal(body, []byte("null")) {
		return nil
	}
	if err := json.Unmarshal(body, &e.Body); err != nil {
		return err
	}
	e.bodyPresent = true
	return nil
}

func (l *requiredWebList[T]) UnmarshalJSON(data []byte) error {
	*l = requiredWebList[T]{}
	l.Present = true
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return nil
	}
	if err := json.Unmarshal(data, &l.Items); err != nil {
		return err
	}
	l.Valid = true
	return nil
}

func (v *flexInt64) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if bytes.Equal(data, []byte("null")) || len(data) == 0 {
		*v = 0
		return nil
	}
	if data[0] == '"' {
		var text string
		if err := json.Unmarshal(data, &text); err != nil {
			return err
		}
		if text == "" {
			*v = 0
			return nil
		}
		parsed, err := strconv.ParseInt(text, 10, 64)
		if err != nil {
			return err
		}
		*v = flexInt64(parsed)
		return nil
	}
	parsed, err := strconv.ParseInt(string(data), 10, 64)
	if err != nil {
		return err
	}
	*v = flexInt64(parsed)
	return nil
}

func (v *flexInt) UnmarshalJSON(data []byte) error {
	var value flexInt64
	if err := value.UnmarshalJSON(data); err != nil {
		return err
	}
	*v = flexInt(value)
	return nil
}
