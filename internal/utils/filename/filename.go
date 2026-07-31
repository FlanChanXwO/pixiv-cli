// Package filename 提供下载路径使用的安全文件名构造逻辑。
package filename

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/utils/text"
)

type FilenameData struct {
	ID        int64
	Author    string
	Title     string
	PageCount int
}

// ValidateTemplate 只接受公开文档承诺的三个占位符。模板直接影响本地文件路径，
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
			case "{id}", "{title}", "{author}":
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

func Sanitize(name string) string {
	re := regexp.MustCompile(`[\\/*?:"<>|]`)
	return re.ReplaceAllString(name, "_")
}

func Generate(data FilenameData, page int, template string) string {
	if template == "" {
		template = "{author} - {title}_{id}"
	}
	name := strings.NewReplacer(
		"{author}", Sanitize(text.DefaultString(data.Author, "UnknownAuthor")),
		"{title}", Sanitize(text.DefaultString(data.Title, "Untitled")),
		"{id}", fmt.Sprint(data.ID),
	).Replace(template)
	// 模板字面量也可能包含路径分隔符；最终文件名必须整体清理，避免下载写出目标目录。
	name = Sanitize(name)
	if data.PageCount > 1 {
		return fmt.Sprintf("%s_p%d", name, page)
	}
	return name
}
