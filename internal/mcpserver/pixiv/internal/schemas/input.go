// Package schemas 集中定义 Pixiv MCP 输入 schema 的小型构造器。
package schemas

// ClosedObject 返回拒绝未知字段的 MCP object schema。
func ClosedObject(properties map[string]any, required []string) map[string]any {
	// JSON Schema 要求 required 为字符串数组。Go 的 nil 切片会编码成 JSON null，
	// MCP 客户端（如 TypeScript SDK 的 ToolSchema）会因此拒绝整个 tools/list 响应，
	// 所以这里必须把 nil 归一化为空数组。
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

// List 返回包含统一逻辑分页字段的 MCP object schema。
func List(properties map[string]any, required ...string) map[string]any {
	all := make(map[string]any, len(properties)+2)
	for name, property := range properties {
		all[name] = property
	}
	all["page"] = map[string]any{
		"type":        "integer",
		"minimum":     1,
		"description": "1-based logical page; requires a positive limit.",
	}
	all["limit"] = map[string]any{
		"type":        "integer",
		"minimum":     0,
		"description": "Maximum logical results; 0 returns all; omit for one logical batch.",
	}
	return ClosedObject(all, required)
}

// PositiveInteger 返回可选或必填正整数共用的 schema property。
func PositiveInteger(description string) map[string]any {
	return map[string]any{
		"type":        "integer",
		"minimum":     1,
		"description": description,
	}
}

// EnumString 返回闭合字符串枚举 property。
func EnumString(description string, values ...string) map[string]any {
	return map[string]any{
		"type":        "string",
		"enum":        values,
		"description": description,
	}
}
