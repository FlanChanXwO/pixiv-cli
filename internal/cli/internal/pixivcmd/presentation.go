package pixivcmd

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/plaintext"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
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
			name = plaintext.SafeLine(name)
		}
		tags = append(tags, name)
	}
	title, author := item.Title, item.User.Name
	if safe {
		title, author = plaintext.SafeLine(title), plaintext.SafeLine(author)
	}
	_, err := fmt.Fprintf(out, "%d %q by %s bookmarks:%d views:%d tags:%s\n", item.ID, title, author, item.TotalBookmarks, item.TotalViews, strings.Join(tags, ","))
	return err
}

// PrintNovels 每本小说输出一行，格式为 ID、标题和作者。
func PrintNovels(out io.Writer, items []pixiv.Novel) error {
	return printNovels(out, items, false)
}

// PrintNovelsSafe 转义小说标题和作者，供直接回显上游文本的用户命令使用。
func PrintNovelsSafe(out io.Writer, items []pixiv.Novel) error {
	return printNovels(out, items, true)
}

func printNovels(out io.Writer, items []pixiv.Novel, safe bool) error {
	for _, item := range items {
		title, author := item.Title, item.User.Name
		if safe {
			title, author = plaintext.SafeLine(title), plaintext.SafeLine(author)
		}
		if _, err := fmt.Fprintf(out, "%d %s — %s\n", item.ID, title, author); err != nil {
			return err
		}
	}
	return nil
}

// PrintUserPreviews 每个用户预览输出一行，格式为 ID 和用户名。
func PrintUserPreviews(out io.Writer, users []pixiv.UserPreview) error {
	return printUserPreviews(out, users, false)
}

// PrintUserPreviewsSafe 转义用户名，供直接回显上游文本的用户命令使用。
func PrintUserPreviewsSafe(out io.Writer, users []pixiv.UserPreview) error {
	return printUserPreviews(out, users, true)
}

func printUserPreviews(out io.Writer, users []pixiv.UserPreview, safe bool) error {
	for _, item := range users {
		name := item.User.Name
		if safe {
			name = plaintext.SafeLine(name)
		}
		if _, err := fmt.Fprintf(out, "%d %s\n", item.User.ID, name); err != nil {
			return err
		}
	}
	return nil
}

// WriteJSONLine 输出一个紧凑 JSON 值并追加换行。
func WriteJSONLine(out io.Writer, value any) error {
	body, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if _, err := out.Write(body); err != nil {
		return err
	}
	_, err = io.WriteString(out, "\n")
	return err
}

// WriteJSONValue 输出一个两空格缩进的 JSON 值并追加换行。
func WriteJSONValue(out io.Writer, value any) error {
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	_, err = out.Write(body)
	return err
}
