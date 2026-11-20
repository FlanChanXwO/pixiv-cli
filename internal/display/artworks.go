package display

import (
	"fmt"
	"io"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/i18n"
	"github.com/FlanChanXwO/pixiv-cli/internal/record"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

// PrintArtworks 每项输出一行标题、作者和收藏数，并转义上游文本。
func PrintArtworks(out io.Writer, language i18n.Locale, items []record.Artwork) error {
	for _, item := range items {
		if err := printArtworkLine(out, language, item); err != nil {
			return err
		}
	}
	return nil
}

// PrintArtworksRanked 在每项前加排名；offset 是本批在整体结果中的起点。
func PrintArtworksRanked(out io.Writer, language i18n.Locale, items []record.Artwork, offset int) error {
	for index, item := range items {
		if _, err := fmt.Fprintf(out, "#%d ", offset+index+1); err != nil {
			return err
		}
		if err := printArtworkLine(out, language, item); err != nil {
			return err
		}
	}
	return nil
}

func printArtworkLine(out io.Writer, language i18n.Locale, item record.Artwork) error {
	_, err := fmt.Fprintf(out, language.Text("output.bookmarks"), item.ID, SafeLine(item.Title), SafeLine(item.User.Name), item.TotalBookmarks)
	return err
}

// PrintArtworkDetail 输出作品详情卡片；上游文本转义为单行。
func PrintArtworkDetail(out io.Writer, language i18n.Locale, item record.Artwork) error {
	tags := make([]string, 0, len(item.Tags))
	for _, tag := range item.Tags {
		tags = append(tags, SafeLine(tag.Name))
	}
	url := ""
	if item.ID > 0 {
		url = fmt.Sprintf("https://www.pixiv.net/artworks/%d", item.ID)
	}
	content := string(item.ContentType)
	switch item.ContentType {
	case pixiv.ContentTypeIllust, pixiv.ContentTypeManga, pixiv.ContentTypeUgoira:
	default:
		content = "unknown"
	}
	content = language.Text("display.type_" + content)
	lines := []string{fmt.Sprintf("%s — %s", SafeLine(item.Title), Identity(item.User.Name, item.User.ID))}
	if url != "" {
		lines = append(lines, url)
	}
	lines = append(lines, fmt.Sprintf(language.Text("format.pages_bookmarks_views"), SafeLine(content), item.PageCount, item.TotalBookmarks, item.TotalViews))
	if len(tags) > 0 {
		lines = append(lines, language.Text("display.tags")+strings.Join(tags, ", "))
	}
	for _, line := range lines {
		if _, err := fmt.Fprintln(out, line); err != nil {
			return err
		}
	}

	if caption := htmlPlainText(item.Caption); caption != "" {
		if _, err := fmt.Fprintf(out, "\n%s\n", caption); err != nil {
			return err
		}
	}
	return nil
}
