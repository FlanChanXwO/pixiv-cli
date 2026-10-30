package display

import (
	"fmt"
	"io"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

// PrintArtworks 转义上游文本，输出作品 URL 和元数据；没有 ID 时省略 URL。
func PrintArtworks(out io.Writer, items []pixiv.Artwork) error {
	for _, item := range items {
		url := ""
		if item.ID > 0 {
			url = fmt.Sprintf("https://www.pixiv.net/artworks/%d", item.ID)
		}
		if _, err := fmt.Fprintln(out, url); err != nil {
			return err
		}
		if err := printArtworkLine(out, item); err != nil {
			return err
		}
	}
	return nil
}

// PrintArtworksRanked 在转义后的每项前加排名；offset 是本批在整体结果中的起点。
func PrintArtworksRanked(out io.Writer, items []pixiv.Artwork, offset int) error {
	for index, item := range items {
		if _, err := fmt.Fprintf(out, "#%d https://www.pixiv.net/artworks/%d\n", offset+index+1, item.ID); err != nil {
			return err
		}
		if err := printArtworkLine(out, item); err != nil {
			return err
		}
	}
	return nil
}

func printArtworkLine(out io.Writer, item pixiv.Artwork) error {
	tags := make([]string, 0, len(item.Tags))
	for _, tag := range item.Tags {
		tags = append(tags, SafeLine(tag.Name))
	}
	_, err := fmt.Fprintf(out, "%d %q by %s bookmarks:%d views:%d tags:%s\n", item.ID, SafeLine(item.Title), SafeLine(item.User.Name), item.TotalBookmarks, item.TotalViews, strings.Join(tags, ","))
	return err
}

// PrintArtworkDetail 输出作品详情卡片；上游文本转义为单行。
func PrintArtworkDetail(out io.Writer, item pixiv.Artwork) error {
	tags := make([]string, 0, len(item.Tags))
	for _, tag := range item.Tags {
		tags = append(tags, SafeLine(tag.Name))
	}
	url := ""
	if item.ID > 0 {
		url = fmt.Sprintf("https://www.pixiv.net/artworks/%d", item.ID)
	}
	for _, line := range []string{
		fmt.Sprintf("url: %s\n", url), fmt.Sprintf("id: %d\n", item.ID), fmt.Sprintf("title: %s\n", SafeLine(item.Title)),
		fmt.Sprintf("author: %s (%d)\n", SafeLine(item.User.Name), item.User.ID), fmt.Sprintf("type: %s\n", SafeLine(string(item.Kind))),
		fmt.Sprintf("page_count: %d\n", item.PageCount), fmt.Sprintf("bookmarks: %d\n", item.TotalBookmarks),
		fmt.Sprintf("views: %d\n", item.TotalViews), fmt.Sprintf("tags: %s\n", strings.Join(tags, ",")),
	} {
		if _, err := io.WriteString(out, line); err != nil {
			return err
		}
	}
	if caption := htmlPlainText(item.Caption); caption != "" {
		if _, err := fmt.Fprintf(out, "caption:\n%s\n", caption); err != nil {
			return err
		}
	}
	return nil
}
