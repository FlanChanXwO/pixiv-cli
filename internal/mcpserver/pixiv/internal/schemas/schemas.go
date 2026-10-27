// Package schemas 构造 Pixiv MCP 工具的输入与输出 JSON Schema。
//
// 包的组成：[ClosedObject]、[List]、[PositiveInteger] 与 [EnumString] 组装输入 schema，
// 各输出 envelope 的 schema 在 output.go。输出 schema 保留 SDK 记录的未知字段，
// 输入 schema 则封闭，未知字段在进入 handler 前被拒绝。
package schemas

// ClosedObject 返回拒绝未知字段的 MCP object schema；required 为 nil 时写成空数组。
func ClosedObject(properties map[string]any, required []string) map[string]any {
	// JSON Schema 要求 required 是字符串数组；nil 切片会编码成 null，部分 MCP 客户端
	// 因此拒绝整个 tools/list 响应。
	if required == nil {
		required = []string{}
	}
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             required,
		"properties":           properties,
	}
}

// List 把 page 与 limit 加入 properties，返回封闭的列表输入 schema。
func List(properties map[string]any, required ...string) map[string]any {
	properties["page"] = map[string]any{
		"type":        "integer",
		"minimum":     1,
		"description": "1-based logical page; requires a positive limit.",
	}
	properties["limit"] = map[string]any{
		"type":        "integer",
		"minimum":     0,
		"description": "Maximum logical results; 0 returns all; omit for one logical batch.",
	}
	return ClosedObject(properties, required)
}

// PositiveInteger 返回最小值为 1 的整数字段，description 会原样出现在 schema 里。
func PositiveInteger(description string) map[string]any {
	return map[string]any{
		"type":        "integer",
		"minimum":     1,
		"description": description,
	}
}

// EnumString 返回只接受 values 中取值的字符串字段。
func EnumString(description string, values ...string) map[string]any {
	return map[string]any{
		"type":        "string",
		"enum":        values,
		"description": description,
	}
}
