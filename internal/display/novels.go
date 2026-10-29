package display

import (
	"fmt"
	"io"

	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

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
			title, author = SafeLine(title), SafeLine(author)
		}
		if _, err := fmt.Fprintf(out, "%d %s — %s\n", item.ID, title, author); err != nil {
			return err
		}
	}
	return nil
}

// PrintNovelDetail 输出小说详情的一行摘要。
func PrintNovelDetail(out io.Writer, item pixiv.Novel) error {
	_, err := fmt.Fprintf(out, "%d %s — %s\n", item.ID, item.Title, item.User.Name)
	return err
}

// PrintNovelContent 输出结构化小说正文。
func PrintNovelContent(out io.Writer, content pixiv.NovelContent) error {
	if _, err := fmt.Fprintf(out, "novel %d: %s\n", content.NovelID, content.Title); err != nil {
		return err
	}
	for _, block := range content.Blocks {
		switch block.Kind {
		case pixiv.NovelBlockParagraph, pixiv.NovelBlockHeader:
			if _, err := fmt.Fprintln(out, block.Text); err != nil {
				return err
			}
		case pixiv.NovelBlockImage:
			if block.Image != nil {
				if _, err := fmt.Fprintf(out, "[image resource %s] %s\n", block.Image.Resource.Ref.String(), block.Image.Caption); err != nil {
					return err
				}
			}
		case pixiv.NovelBlockFile:
			if block.File != nil {
				if _, err := fmt.Fprintf(out, "[file %s resource %s] %s\n", block.File.Filename, block.File.Resource.Ref.String(), block.File.Caption); err != nil {
					return err
				}
			}
		case pixiv.NovelBlockUnknown:
			if block.Unknown != nil {
				if _, err := fmt.Fprintf(out, "[unknown block %s]\n", block.Unknown.RawType); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
