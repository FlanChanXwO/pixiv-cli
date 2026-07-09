package utils

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/utils/text"
)

type FilenameData struct {
	ID        int64
	Author    string
	Title     string
	PageCount int
}

func SanitizeFilename(name string) string {
	re := regexp.MustCompile(`[\\/*?:"<>|]`)
	return re.ReplaceAllString(name, "_")
}

func GenerateFilename(data FilenameData, page int, template string) string {
	if template == "" {
		template = "{author} - {title}_{id}"
	}
	name := strings.NewReplacer(
		"{author}", SanitizeFilename(text.DefaultString(data.Author, "UnknownAuthor")),
		"{title}", SanitizeFilename(text.DefaultString(data.Title, "Untitled")),
		"{id}", fmt.Sprint(data.ID),
	).Replace(template)
	// 模板字面量也可能包含路径分隔符；最终文件名必须整体清理，避免下载写出目标目录。
	name = SanitizeFilename(name)
	if data.PageCount > 1 {
		return fmt.Sprintf("%s_p%d", name, page)
	}
	return name
}

func Deduplicate(ids []int64) []int64 {
	seen := make(map[int64]struct{}, len(ids))
	unique := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		unique = append(unique, id)
	}
	slices.Sort(unique)
	return unique
}
