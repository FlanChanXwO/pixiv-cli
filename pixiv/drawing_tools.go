package pixiv

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/FlanChanXwO/pixiv-cli/internal/buildinfo"
	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
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

func validDrawingTool(value string) bool {
	if value == "" {
		return true
	}
	for _, tool := range supportedDrawingTools {
		if value == tool {
			return true
		}
	}
	return false
}

func invalidDrawingToolError(value string) error {
	catalogURL := drawingToolCatalogURL()
	message := "unsupported drawing tool; see " + catalogURL
	if suggestion, ok := drawingToolSuggestion(value); ok {
		message = fmt.Sprintf("unsupported drawing tool; did you mean %q? See %s", suggestion, catalogURL)
	}
	err := newError(CodeInvalidArgument, OperationSearchIllust, "", false, 0, 0, errors.New(message))
	err.detail = message
	return err
}

// drawingToolCatalogURL 使 SDK、CLI 与 MCP 在稳定版本中都指向同一不可变文档。
// 本地开发构建尚无 tag，因此明确链接 main 的当前参考页。
func drawingToolCatalogURL() string {
	ref := buildinfo.Current().Version
	if !strings.HasPrefix(ref, "v") {
		ref = "main"
	}
	return "https://github.com/FlanChanXwO/pixiv-cli/blob/" + ref + "/docs/en/cli-reference.md#drawing-tool-catalog"
}

// drawingToolSuggestion 只对可唯一判定的一次编辑错误提示名称。严格校验仍在
// validDrawingTool 中执行；例如含混前缀 "pho" 不会被猜测为 Photoshop。
func drawingToolSuggestion(value string) (string, bool) {
	return drawingToolSuggestionFromCatalog(value, supportedDrawingTools)
}

func drawingToolSuggestionFromCatalog(value string, catalog []string) (string, bool) {
	needle := drawingToolComparisonKey(value)
	if needle == "" {
		return "", false
	}
	var suggestion string
	for _, tool := range catalog {
		candidate := drawingToolComparisonKey(tool)
		if candidate == needle {
			if suggestion != "" && suggestion != tool {
				return "", false
			}
			suggestion = tool
			continue
		}
		if damerauLevenshteinOne(needle, candidate) {
			if suggestion != "" && suggestion != tool {
				return "", false
			}
			suggestion = tool
		}
	}
	return suggestion, suggestion != ""
}

func drawingToolComparisonKey(value string) string {
	value = cases.Fold().String(norm.NFC.String(strings.TrimSpace(value)))
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || r == '-' || r == '_' || r == '.' {
			return -1
		}
		return r
	}, value)
}

// damerauLevenshteinOne 判断两个 rune 序列是否只差一次插入、删除、替换或相邻换位。
func damerauLevenshteinOne(left, right string) bool {
	a, b := []rune(left), []rune(right)
	if len(a) == len(b) {
		differences := make([]int, 0, 2)
		for index := range a {
			if a[index] != b[index] {
				differences = append(differences, index)
				if len(differences) > 2 {
					return false
				}
			}
		}
		switch len(differences) {
		case 1:
			return true
		case 2:
			first, second := differences[0], differences[1]
			return second == first+1 && a[first] == b[second] && a[second] == b[first]
		default:
			return false
		}
	}
	if len(a)+1 == len(b) {
		a, b = b, a
	}
	if len(a) != len(b)+1 {
		return false
	}
	for index := 0; index < len(b); index++ {
		if a[index] != b[index] {
			return string(a[index+1:]) == string(b[index:])
		}
	}
	return true
}
