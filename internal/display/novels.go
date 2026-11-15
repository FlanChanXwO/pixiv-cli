package display

import (
	"fmt"
	"io"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/record"
)

// PrintNovels 转义小说标题和作者，每本小说输出一行。
func PrintNovels(out io.Writer, items []record.Novel) error {
	for _, item := range items {
		if _, err := fmt.Fprintf(out, "%d  %s — %s  (%d bookmarks)\n", item.ID, SafeLine(item.Title), SafeLine(item.User.Name), item.TotalBookmarks); err != nil {
			return err
		}
	}
	return nil
}

// PrintNovelDetail 输出小说详情卡片，并转义上游文本。
func PrintNovelDetail(out io.Writer, item record.Novel) error {
	tags := make([]string, 0, len(item.Tags))
	for _, tag := range item.Tags {
		tags = append(tags, SafeLine(tag.Name))
	}
	_, err := fmt.Fprintf(out, "%s — %s\nhttps://www.pixiv.net/novel/show.php?id=%d\nnovel · %d characters · %d bookmarks · %d views\n", SafeLine(item.Title), Identity(item.User.Name, item.User.ID), item.ID, item.TextLength, item.TotalBookmarks, item.TotalViews)
	if err != nil {
		return err
	}
	if len(tags) > 0 {
		if _, err := fmt.Fprintf(out, "Tags: %s\n", strings.Join(tags, ", ")); err != nil {
			return err
		}
	}
	if caption := htmlPlainText(item.Caption); caption != "" {
		_, err = fmt.Fprintf(out, "\n%s\n", caption)
	}
	return err
}
