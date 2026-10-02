package protocol

import (
	"bytes"
	"encoding/json"
)

// 本文件收口 endpoint 层多处重复的"必需字段存在性解码"。
//
// Pixiv App API 的响应里，一个字段可能：完全缺失、显式为 null、是合法空集合、
// 是合法负载，或者类型错误。endpoint 需要区分这些情形，才能决定"响应完整"还是
// "应当拒绝"。这里共享存在性解码（历史实现差异见下文），
// 由各 endpoint 继续保留自己的完整性判断与上下文错误。
//
// 语义：
//   - Present 表达"字段出现在 JSON 中"，在 UnmarshalJSON 被调用时即为 true；
//   - Valid 表达"字段存在且成功解码为 T"；
//   - null 属"存在但无效"（Present=true, Valid=false），不是缺失；
//   - 每次 UnmarshalJSON 调用先重置状态；解码失败可能留下本次部分数据，须检查错误与 Valid。
//   - 缺失字段不会调用字段 decoder；复用外层 DTO 时，调用方必须先清零或新建外层值。
//
// 关于两个类型的收口来源（精确说明，勿简化为"全部逐字相同"）：
//   - RequiredList 来自 26 处**逐字相同**的局部实现。
//   - RequiredObject 来自 3 处，其中 2 处（artwork/detail、artwork/trending）多一个
//     分支：当载荷不是以 '{' 开头时直接 json.Unmarshal 到 Value 且**不设置 Valid**。
//     对目前七种生产 DTO（ugoiraMetadataDTO、ugoiraZipURLsDTO、illustDTO、
//     userDTO、profileDTO、profilePublicityDTO、workspaceDTO），历史差分覆盖了
//     missing/null/空对象/合法对象/错字段/数组/字符串/布尔/数字及外层复用，未见差异。
//     因此统一采用不带该分支的实现；这不保证任意 T（尤其自定义 decoder）都等价。
//     若未来新增 RequiredObject 实例，须重新评估该差异。

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
