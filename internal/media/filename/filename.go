// Package filename 构造下载路径使用的安全文件名与目录模板。
package filename

import (
	"fmt"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/plaintext"
)

// Fields 是一次文件名或目录模板渲染可用的作品字段。
type Fields struct {
	ID         int64
	Author     string
	AuthorID   int64
	Title      string
	CreateDate string
	Tags       []string
	PageCount  int
}

var unsafeNameCharacters = regexp.MustCompile(`[\\/*?:"<>|]`)

// ValidateTemplate 只接受公开文档承诺的下载占位符。模板直接影响本地文件路径，
// 因此未知占位符和不配对花括号必须在网络请求前明确拒绝，而不能作为字面量落盘。
func ValidateTemplate(template string) error {
	for index := 0; index < len(template); {
		switch template[index] {
		case '{':
			end := strings.IndexByte(template[index+1:], '}')
			if end < 0 {
				return fmt.Errorf("filename template contains an unmatched '{'")
			}
			end += index + 1
			placeholder := template[index : end+1]
			switch placeholder {
			case "{id}", "{title}", "{author}", "{author_id}", "{date}", "{tags}", "{num}":
				index = end + 1
			default:
				return fmt.Errorf("filename template contains unsupported placeholder %q", placeholder)
			}
		case '}':
			return fmt.Errorf("filename template contains an unmatched '}'")
		default:
			index++
		}
	}
	return nil
}

// hasPlaceholder 报告已经通过语法校验的模板是否引用某个占位符。
func hasPlaceholder(template, placeholder string) bool {
	return strings.Contains(template, placeholder)
}

// Sanitize 把路径分隔符和各平台不允许出现在文件名中的字符替换为下划线。
func Sanitize(name string) string {
	return unsafeNameCharacters.ReplaceAllString(name, "_")
}

// GenerateChecked 生成安全文件名，并在模板实际使用日期时校验 Pixiv 时间。{num}
// 是 0-based 页码；未显式出现时保留历史多页 _pN 后缀。
func GenerateChecked(data Fields, page int, template string) (string, error) {
	return generateChecked(data, page, template, true)
}

// generateChecked 在 filename 和 directory template 间复用同一占位符渲染逻辑。
// 只有最终文件名需要历史兼容的多页 _pN 后缀；目录层级绝不能凭空追加页后缀。
func generateChecked(data Fields, page int, template string, appendPageSuffix bool) (string, error) {
	if template == "" {
		template = "{author} - {title}_{id}"
	}
	if err := ValidateTemplate(template); err != nil {
		return "", err
	}
	date := ""
	if hasPlaceholder(template, "{date}") {
		parsed, err := time.Parse(time.RFC3339, data.CreateDate)
		if err != nil {
			return "", fmt.Errorf("filename template requires a valid create date")
		}
		date = parsed.Format("2006-01-02")
	}
	name := strings.NewReplacer(
		"{author}", Sanitize(plaintext.DefaultString(data.Author, "UnknownAuthor")),
		"{title}", Sanitize(plaintext.DefaultString(data.Title, "Untitled")),
		"{id}", fmt.Sprint(data.ID),
		"{author_id}", fmt.Sprint(data.AuthorID),
		"{date}", Sanitize(date),
		"{tags}", Sanitize(strings.Join(data.Tags, ",")),
		"{num}", fmt.Sprint(page),
	).Replace(template)
	// 模板字面量也可能包含路径分隔符；最终文件名必须整体清理，避免下载写出目标目录。
	name = Sanitize(name)
	if appendPageSuffix && data.PageCount > 1 && !hasPlaceholder(template, "{num}") {
		return fmt.Sprintf("%s_p%d", name, page), nil
	}
	return name, nil
}

// ValidateDirectoryTemplate 在网络请求前只校验模板语法和静态路径结构。动态字段
// （例如空 tags）只能在取得具体作品后判断，不能用零值作品提前误判。
func ValidateDirectoryTemplate(template string) error {
	template = strings.TrimSpace(template)
	if template == "" {
		return nil
	}
	if strings.HasPrefix(template, "/") || strings.HasPrefix(template, `\`) || path.IsAbs(template) {
		return fmt.Errorf("directory template must be relative")
	}
	if err := ValidateTemplate(template); err != nil {
		return err
	}
	for _, segment := range strings.Split(template, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return fmt.Errorf("directory template contains an unsafe path segment")
		}
	}
	return nil
}

// BuildRelativeDirectory 渲染下载目录模板。路径一律使用 / 表示层级，渲染后的
// 每段单独清理，禁止空段、绝对路径和 . / ..，避免从下载目录逃逸。
func BuildRelativeDirectory(template string, data Fields, page int) (string, error) {
	template = strings.TrimSpace(template)
	if template == "" {
		return "", nil
	}
	if err := ValidateDirectoryTemplate(template); err != nil {
		return "", err
	}
	segments := strings.Split(template, "/")
	rendered := make([]string, len(segments))
	for index, segment := range segments {
		if segment == "" || segment == "." || segment == ".." {
			return "", fmt.Errorf("directory template contains an unsafe path segment")
		}
		value, err := generateChecked(data, page, segment, false)
		if err != nil {
			return "", err
		}
		if value == "" || value == "." || value == ".." {
			return "", fmt.Errorf("directory template renders an unsafe path segment")
		}
		rendered[index] = value
	}
	return path.Join(rendered...), nil
}
