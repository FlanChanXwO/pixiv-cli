package search

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
)

// drawingToolsJSON 是随版本发布的静态目录数据，而不是散落在 Go 常量中的业务值。
//
//go:embed drawing-tools.json
var drawingToolsJSON []byte

// supportedDrawingTools 只保存由 JSON 解析并校验后的目录快照。它只用于精确搜索
// 筛选；目录变更必须随版本更新并由真实搜索验收。
var supportedDrawingTools = mustLoadDrawingTools(drawingToolsJSON)

func mustLoadDrawingTools(payload []byte) []string {
	var tools []string
	if err := json.Unmarshal(payload, &tools); err != nil {
		panic(fmt.Sprintf("invalid embedded drawing-tool catalog: %v", err))
	}
	if len(tools) == 0 {
		panic("invalid embedded drawing-tool catalog: empty")
	}
	seen := make(map[string]struct{}, len(tools))
	for _, tool := range tools {
		if strings.TrimSpace(tool) == "" {
			panic("invalid embedded drawing-tool catalog: empty tool")
		}
		if _, duplicate := seen[tool]; duplicate {
			panic("invalid embedded drawing-tool catalog: duplicate tool")
		}
		seen[tool] = struct{}{}
	}
	return tools
}

// SupportedDrawingTools 返回本版本支持的 Pixiv 制图工具精确名称。结果是副本，
// 调用方可自由修改，且不会影响后续搜索校验。
func SupportedDrawingTools() []string { return append([]string(nil), supportedDrawingTools...) }
