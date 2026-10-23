// Package record owns the stable Pixiv record projection shared by CLI and MCP.
package record

// Record 是可在 CLI 管道和 MCP 之间共享的 Pixiv 实体 JSON 记录。
// 它保留源对象和外部程序提供的未知字段，同时固定顶层 id、type、url。
type Record struct {
	fields map[string]any
}

// ID 返回记录的稳定字符串标识。
func (r Record) ID() string {
	return stringField(r.fields["id"])
}

// Type 返回记录的实体类型。
func (r Record) Type() string {
	return stringField(r.fields["type"])
}

// URL 返回记录的规范 Pixiv URL。
func (r Record) URL() string {
	return stringField(r.fields["url"])
}

func stringField(raw any) string {
	value, _ := raw.(string)
	return value
}
