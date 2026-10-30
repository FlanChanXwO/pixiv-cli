package display

import (
	"fmt"
	"io"

	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

// PrintNovels 转义小说标题和作者，每本小说输出一行。
func PrintNovels(out io.Writer, items []pixiv.Novel) error {
	for _, item := range items {
		if err := PrintNovelDetail(out, item); err != nil {
			return err
		}
	}
	return nil
}

// PrintNovelDetail 输出小说详情的一行摘要。
func PrintNovelDetail(out io.Writer, item pixiv.Novel) error {
	_, err := fmt.Fprintf(out, "%d %s — %s\n", item.ID, SafeLine(item.Title), SafeLine(item.User.Name))
	return err
}

// PrintNovelContent 输出结构化小说正文。
func PrintNovelContent(out io.Writer, content pixiv.NovelContent) error {
	if _, err := fmt.Fprintf(out, "novel %d: %s\n", content.NovelID, SafeLine(content.Title)); err != nil {
		return err
	}
	for _, block := range content.Blocks {
		switch block.Kind {
		case pixiv.NovelBlockParagraph, pixiv.NovelBlockHeader:
			if _, err := fmt.Fprintln(out, SafeLine(block.Text)); err != nil {
				return err
			}
		case pixiv.NovelBlockImage:
			if block.Image != nil {
				if _, err := fmt.Fprintf(out, "[image resource %s] %s\n", block.Image.Resource.Ref.String(), SafeLine(block.Image.Caption)); err != nil {
					return err
				}
			}
		case pixiv.NovelBlockFile:
			if block.File != nil {
				if _, err := fmt.Fprintf(out, "[file %s resource %s] %s\n", SafeLine(block.File.Filename), block.File.Resource.Ref.String(), SafeLine(block.File.Caption)); err != nil {
					return err
				}
			}
		case pixiv.NovelBlockUnknown:
			if block.Unknown != nil {
				if _, err := fmt.Fprintf(out, "[unknown block %s]\n", SafeLine(block.Unknown.RawType)); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
