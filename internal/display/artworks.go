package display

import (
	"fmt"
	"io"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

// PrintArtworks 输出 timeline、MyPixiv、收藏和推荐共用的作品格式：URL 后跟一行元数据。
func PrintArtworks(out io.Writer, items []pixiv.Artwork) error {
	return printArtworks(out, items, false)
}

// PrintArtworksRanked 在每项前加排名（从 #1 开始）；offset 是本批在整体结果中的起点。
func PrintArtworksRanked(out io.Writer, items []pixiv.Artwork, offset int) error {
	return printRankedArtworks(out, items, offset, false)
}

// PrintArtworksSearch 为没有 ID 的作品省略 URL，与搜索展示保持一致。
func PrintArtworksSearch(out io.Writer, items []pixiv.Artwork) error {
	return printArtworks(out, items, true)
}

// PrintArtworksSafe 转义上游返回的标题、作者和标签，避免用户命令把控制字符写进终端。
func PrintArtworksSafe(out io.Writer, items []pixiv.Artwork) error {
	return printSafeArtworks(out, items)
}

func printArtworks(out io.Writer, items []pixiv.Artwork, searchURL bool) error {
	for _, item := range items {
		url := fmt.Sprintf("https://www.pixiv.net/artworks/%d", item.ID)
		if searchURL {
			url = ""
			if item.ID > 0 {
				url = fmt.Sprintf("https://www.pixiv.net/artworks/%d", item.ID)
			}
		}
		if _, err := fmt.Fprintf(out, "%s\n", url); err != nil {
			return err
		}
		if err := printArtworkLine(out, item, false); err != nil {
			return err
		}
	}
	return nil
}

func printRankedArtworks(out io.Writer, items []pixiv.Artwork, offset int, safe bool) error {
	for index, item := range items {
		if _, err := fmt.Fprintf(out, "#%d https://www.pixiv.net/artworks/%d\n", offset+index+1, item.ID); err != nil {
			return err
		}
		if err := printArtworkLine(out, item, safe); err != nil {
			return err
		}
	}
	return nil
}

func printSafeArtworks(out io.Writer, items []pixiv.Artwork) error {
	for _, item := range items {
		if _, err := fmt.Fprintf(out, "https://www.pixiv.net/artworks/%d\n", item.ID); err != nil {
			return err
		}
		if err := printArtworkLine(out, item, true); err != nil {
			return err
		}
	}
	return nil
}

// printArtworkLine 输出元数据行；safe 只转义上游文本，URL 和数字保持原样。
func printArtworkLine(out io.Writer, item pixiv.Artwork, safe bool) error {
	tags := make([]string, 0, len(item.Tags))
	for _, tag := range item.Tags {
		name := tag.Name
		if safe {
			name = SafeLine(name)
		}
		tags = append(tags, name)
	}
	title, author := item.Title, item.User.Name
	if safe {
		title, author = SafeLine(title), SafeLine(author)
	}
	_, err := fmt.Fprintf(out, "%d %q by %s bookmarks:%d views:%d tags:%s\n", item.ID, title, author, item.TotalBookmarks, item.TotalViews, strings.Join(tags, ","))
	return err
}

// PrintArtworkDetail 输出作品详情卡片；上游文本原样进入终端。
func PrintArtworkDetail(out io.Writer, item pixiv.Artwork) error {
	tags := make([]string, 0, len(item.Tags))
	for _, tag := range item.Tags {
		tags = append(tags, tag.Name)
	}
	url := ""
	if item.ID > 0 {
		url = fmt.Sprintf("https://www.pixiv.net/artworks/%d", item.ID)
	}
	for _, line := range []string{
		fmt.Sprintf("url: %s\n", url), fmt.Sprintf("id: %d\n", item.ID), fmt.Sprintf("title: %s\n", item.Title),
		fmt.Sprintf("author: %s (%d)\n", item.User.Name, item.User.ID), fmt.Sprintf("type: %s\n", string(item.Kind)),
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
