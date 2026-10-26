// Package record owns the stable Pixiv record projection shared by CLI and MCP.
package record

// Record 是可在 CLI 管道和 MCP 之间共享的 Pixiv 实体 JSON 记录。
// 它保留源对象和外部程序提供的未知字段，同时固定顶层 id、type、url。
// 零值尚未初始化；读取身份字段前应通过构造或 JSON 解析入口建立记录。
type Record struct {
	fields map[string]any
}

// ID 返回记录的稳定字符串标识。
func (r Record) ID() string {
	return r.fields["id"].(string)
}

// Type 返回记录的实体类型。
func (r Record) Type() string {
	return r.fields["type"].(string)
}

// URL 返回记录的规范 Pixiv URL。
func (r Record) URL() string {
	return r.fields["url"].(string)
}
