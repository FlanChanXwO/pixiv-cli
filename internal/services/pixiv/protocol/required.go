package protocol

import (
	"bytes"
	"encoding/json"
)

// 本文件收口 endpoint 层多处重复的"必需字段存在性解码"。
//
// Pixiv App API 的响应里，一个字段可能：完全缺失、显式为 null、是合法空集合、
// 是合法负载，或者类型错误。endpoint 需要区分这些情形，才能决定"响应完整"还是
// "应当拒绝"。收口前每个 endpoint 各自复制了一份完全相同的实现，因此这里提供
// 两个共享类型，由各 endpoint 继续保留自己的完整性判断与上下文错误。
//
// 语义（与收口前的局部实现逐字一致）：
//   - Present 表达"字段出现在 JSON 中"，在 UnmarshalJSON 被调用时即为 true；
//   - Valid 表达"字段存在且成功解码为 T"；
//   - null 属"存在但无效"（Present=true, Valid=false），不是缺失；
//   - UnmarshalJSON 整体重置状态，因此对同一个值重新解码不会残留旧结果。

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
