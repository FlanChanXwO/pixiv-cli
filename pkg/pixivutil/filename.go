package pixivutil

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
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
		"{author}", SanitizeFilename(fallback(data.Author, "UnknownAuthor")),
		"{title}", SanitizeFilename(fallback(data.Title, "Untitled")),
		"{id}", fmt.Sprint(data.ID),
	).Replace(template)
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

func fallback(value, backup string) string {
	if value == "" {
		return backup
	}
	return value
}
