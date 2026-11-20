package display

import (
	"fmt"
	"io"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/i18n"
	"github.com/FlanChanXwO/pixiv-cli/internal/record"
)

// PrintNovels 转义小说标题和作者，每本小说输出一行。
func PrintNovels(out io.Writer, language i18n.Locale, items []record.Novel) error {
	for _, item := range items {
		if _, err := fmt.Fprintf(out, language.Text("output.bookmarks"), item.ID, SafeLine(item.Title), SafeLine(item.User.Name), item.TotalBookmarks); err != nil {
			return err
		}
	}
	return nil
}

// PrintNovelDetail 输出小说详情卡片，并转义上游文本。
func PrintNovelDetail(out io.Writer, language i18n.Locale, item record.Novel) error {
	tags := make([]string, 0, len(item.Tags))
	for _, tag := range item.Tags {
		tags = append(tags, SafeLine(tag.Name))
	}
	_, err := fmt.Fprintf(out, language.Text("output.https_www_pixiv_net_novel_show_php_id"), SafeLine(item.Title), Identity(item.User.Name, item.User.ID), item.ID, item.TextLength, item.TotalBookmarks, item.TotalViews)
	if err != nil {
		return err
	}
	if len(tags) > 0 {
		if _, err := fmt.Fprintf(out, language.Text("output.tags"), strings.Join(tags, ", ")); err != nil {
			return err
		}
	}
	if caption := htmlPlainText(item.Caption); caption != "" {
		_, err = fmt.Fprintf(out, "\n%s\n", caption)
	}
	return err
}
