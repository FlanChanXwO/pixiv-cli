package pixiv

import (
	"fmt"
	"strings"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/ast"
	"github.com/expr-lang/expr/parser"
	"github.com/expr-lang/expr/vm"
)

// IllustFilter 是编译后的本地作品筛选器。它只可由 CompileIllustFilter 创建，
// 以确保 CLI、MCP 与 SDK 共用同一份字段白名单和语法边界。
type IllustFilter struct {
	expression string
	program    *vm.Program
}

// illustFilterEnvironment 是 Expr 的静态、无方法环境。字段名是公开筛选契约，
// 不直接暴露 Illust，避免表达式获得 URL、嵌套对象或反射访问能力。
type illustFilterEnvironment struct {
	ID            int64    `expr:"id"`
	UserID        int64    `expr:"userId"`
	UserName      string   `expr:"userName"`
	Type          string   `expr:"type"`
	Title         string   `expr:"title"`
	CreateDate    string   `expr:"createDate"`
	PageCount     int      `expr:"pageCount"`
	BookmarkCount int      `expr:"bookmarkCount"`
	ViewCount     int      `expr:"viewCount"`
	XRestrict     int      `expr:"xRestrict"`
	AIType        int      `expr:"aiType"`
	Width         int      `expr:"width"`
	Height        int      `expr:"height"`
	Tags          []string `expr:"tags"`
	Tools         []string `expr:"tools"`

	Rating      string `expr:"rating"`
	AIMode      string `expr:"aiMode"`
	AspectRatio string `expr:"aspectRatio"`
	Resolution  string `expr:"resolution"`
	DrawTool    string `expr:"drawTool"`
}

// CompileIllustFilter 编译一个副作用为零的作品筛选表达式。语法仅限比较、布尔
// 组合、数组字面量与 any/all；错误会包含表达式位置，调用方必须在发起请求前处理它。
func CompileIllustFilter(expression string) (*IllustFilter, error) {
	expression = strings.TrimSpace(expression)
	if expression == "" {
		return nil, fmt.Errorf("filter expression is empty")
	}
	tree, err := parser.Parse(expression)
	if err != nil {
		return nil, fmt.Errorf("invalid filter expression: %w", err)
	}
	validator := illustFilterASTValidator{}
	ast.Walk(&tree.Node, &validator)
	if validator.err != nil {
		return nil, validator.err
	}
	program, err := expr.Compile(expression, expr.Env(illustFilterEnvironment{}), expr.AsBool())
	if err != nil {
		return nil, fmt.Errorf("invalid filter expression: %w", err)
	}
	return &IllustFilter{expression: expression, program: program}, nil
}

// Match 判断作品是否满足编译后的表达式。正常的 SDK 作品模型总能构造完整的
// 受限环境；若 Expr 运行时报告错误，会原样作为筛选失败返回而不是默认为不匹配。
func (f *IllustFilter) Match(illust Illust) (bool, error) {
	if f == nil || f.program == nil {
		return false, fmt.Errorf("illust filter is not initialized")
	}
	value, err := expr.Run(f.program, illustFilterValues(illust))
	if err != nil {
		return false, fmt.Errorf("evaluate filter: %w", err)
	}
	matched, ok := value.(bool)
	if !ok {
		return false, fmt.Errorf("evaluate filter: expected boolean result")
	}
	return matched, nil
}

func illustFilterValues(illust Illust) illustFilterEnvironment {
	tags := make([]string, 0, len(illust.Tags))
	for _, tag := range illust.Tags {
		tags = append(tags, tag.Name)
	}
	tools := append([]string(nil), illust.Tools...)
	return illustFilterEnvironment{
		ID:            illust.ID,
		UserID:        illust.User.ID,
		UserName:      illust.User.Name,
		Type:          illust.Type,
		Title:         illust.Title,
		CreateDate:    illust.CreateDate,
		PageCount:     illust.PageCount,
		BookmarkCount: illust.TotalBookmarks,
		ViewCount:     illust.TotalView,
		XRestrict:     illust.XRestrict,
		AIType:        illust.AIType,
		Width:         illust.Width,
		Height:        illust.Height,
		Tags:          tags,
		Tools:         tools,
		Rating:        illustFilterRating(illust.XRestrict),
		AIMode:        illustFilterAIMode(illust.AIType),
		AspectRatio:   illustFilterAspectRatio(illust.Width, illust.Height),
		Resolution:    illustFilterResolution(illust.Width, illust.Height),
		DrawTool:      illustFilterDrawTool(tools),
	}
}

func illustFilterRating(xRestrict int) string {
	switch xRestrict {
	case 0:
		return string(SearchRatingSFW)
	case 1:
		return string(SearchRatingR18)
	case 2:
		return string(SearchRatingR18G)
	default:
		return "unknown"
	}
}

func illustFilterAIMode(aiType int) string {
	if aiType == 2 {
		return string(SearchAIModeOnly)
	}
	return string(SearchAIModeExclude)
}

func illustFilterAspectRatio(width, height int) string {
	if width <= 0 || height <= 0 {
		return string(SearchAspectRatioAll)
	}
	if width == height {
		return string(SearchAspectRatioSquare)
	}
	if width > height {
		return string(SearchAspectRatioLandscape)
	}
	return string(SearchAspectRatioPortrait)
}

func illustFilterResolution(width, height int) string {
	// 与 SearchIllustFilters.Resolution 的 App/Web 参数严格保持同一边界：两边
	// 都至少 3000 为 high，均在 1000..2999 为 medium，均不超过 999 为 low。
	// 缺失尺寸不能诚实地归入任何 tier，故保持 all。
	if width <= 0 || height <= 0 {
		return string(SearchResolutionAll)
	}
	switch {
	case width >= 3000 && height >= 3000:
		return string(SearchResolutionHigh)
	case width >= 1000 && width <= 2999 && height >= 1000 && height <= 2999:
		return string(SearchResolutionMedium)
	case width <= 999 && height <= 999:
		return string(SearchResolutionLow)
	default:
		return string(SearchResolutionAll)
	}
}

func illustFilterDrawTool(tools []string) string {
	if len(tools) == 1 {
		return tools[0]
	}
	return ""
}

type illustFilterASTValidator struct {
	err error
}

func (v *illustFilterASTValidator) Visit(node *ast.Node) {
	if v.err != nil || node == nil || *node == nil {
		return
	}
	fail := func(reason string) {
		location := (*node).Location()
		v.err = fmt.Errorf("invalid filter at column %d: %s", location.From+1, reason)
	}
	switch n := (*node).(type) {
	case *ast.IdentifierNode:
		if !allowedIllustFilterFields[n.Value] {
			fail(fmt.Sprintf("unknown field %q", n.Value))
		}
	case *ast.IntegerNode, *ast.StringNode, *ast.BoolNode, *ast.ArrayNode, *ast.PredicateNode, *ast.PointerNode:
		return
	case *ast.UnaryNode:
		if n.Operator != "not" {
			fail(fmt.Sprintf("operator %q is not allowed", n.Operator))
		}
	case *ast.BinaryNode:
		switch n.Operator {
		case "==", "!=", "<", "<=", ">", ">=", "in", "not in", "and", "or":
		default:
			fail(fmt.Sprintf("operator %q is not allowed", n.Operator))
		}
	case *ast.BuiltinNode:
		if n.Name != "any" && n.Name != "all" {
			fail(fmt.Sprintf("function %q is not allowed", n.Name))
		}
	default:
		fail(fmt.Sprintf("syntax %T is not allowed", *node))
	}
}

var allowedIllustFilterFields = map[string]bool{
	"id": true, "userId": true, "userName": true, "type": true, "title": true,
	"createDate": true, "pageCount": true, "bookmarkCount": true, "viewCount": true,
	"xRestrict": true, "aiType": true, "width": true, "height": true, "tags": true,
	"tools": true, "rating": true, "aiMode": true, "aspectRatio": true,
	"resolution": true, "drawTool": true,
}
