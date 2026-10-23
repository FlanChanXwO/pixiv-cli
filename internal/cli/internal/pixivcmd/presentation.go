// Line-oriented presenters shared by Pixiv commands. Variants that escape
// upstream text keep a separate entry point so their output stays identical.

package pixivcmd

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/plaintext"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

// PrintArtworks writes the artwork list format shared by timeline, MyPixiv,
// bookmarks, and recommendations.
func PrintArtworks(out io.Writer, items []pixiv.Artwork) error {
	return printArtworks(out, items, false)
}

// PrintArtworksRanked prefixes each artwork with its rank, including #1.
func PrintArtworksRanked(out io.Writer, items []pixiv.Artwork, offset int) error {
	return printRankedArtworks(out, items, offset, false)
}

// PrintArtworksSearch omits the URL for an artwork without an ID, matching the
// search presenter.
func PrintArtworksSearch(out io.Writer, items []pixiv.Artwork) error {
	return printArtworks(out, items, true)
}

// PrintArtworksSafe escapes upstream text for the user command's terminal
// output.
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

// printArtworkLine renders the metadata line; only upstream text is escaped.
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

// PrintNovels writes one line per novel.
func PrintNovels(out io.Writer, items []pixiv.Novel) error {
	return printNovels(out, items, false)
}

// PrintNovelsSafe escapes upstream text for the user command.
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

// PrintUserPreviews writes one line per user preview.
func PrintUserPreviews(out io.Writer, users []pixiv.UserPreview) error {
	return printUserPreviews(out, users, false)
}

// PrintUserPreviewsSafe escapes upstream text for the user command.
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

// WriteJSONLine writes one compact JSON value followed by a newline.
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

// WriteJSONValue writes one indented JSON value followed by a newline.
func WriteJSONValue(out io.Writer, value any) error {
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	_, err = out.Write(body)
	return err
}
