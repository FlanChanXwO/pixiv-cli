package api

import (
	"bytes"
	"encoding/json"
)

// Present 记录响应字段是否出现，Valid 记录 T 是否解码成功。显式 null 记为出现但无效；
// 哪些字段必需、失败如何报告由端点 owner 决定。复用外层 wire 值前必须清零：缺失字段
// 不会调用这里的解码方法。

// RequiredList 解码一个必需数组字段，并保留其存在性与有效性。
type RequiredList[T any] struct {
	Items   []T
	Present bool
	Valid   bool
}

// UnmarshalJSON 先把字段标记为已出现；数组解码失败时返回错误，Present 保持 true 供
// 端点报告上下文。
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

// UnmarshalJSON 先把字段标记为已出现；对象解码失败时返回错误，Present 保持 true 供
// 端点报告上下文。
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
