package search

import (
	"fmt"
	"io"
	"strings"

	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

func printArtworks(out io.Writer, items []pixiv.Artwork) error {
	for _, item := range items {
		url := ""
		if item.ID > 0 {
			url = fmt.Sprintf("https://www.pixiv.net/artworks/%d", item.ID)
		}
		if _, err := fmt.Fprintf(out, "%s\n", url); err != nil {
			return err
		}
		tags := make([]string, 0, len(item.Tags))
		for _, tag := range item.Tags {
			tags = append(tags, tag.Name)
		}
		if _, err := fmt.Fprintf(out, "%d %q by %s bookmarks:%d views:%d tags:%s\n", item.ID, item.Title, item.User.Name, item.TotalBookmarks, item.TotalViews, strings.Join(tags, ",")); err != nil {
			return err
		}
	}
	return nil
}

func printNovels(out io.Writer, items []pixiv.Novel) error {
	for _, item := range items {
		if _, err := fmt.Fprintf(out, "%d %s — %s\n", item.ID, item.Title, item.User.Name); err != nil {
			return err
		}
	}
	return nil
}
