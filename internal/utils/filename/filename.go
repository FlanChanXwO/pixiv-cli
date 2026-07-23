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
