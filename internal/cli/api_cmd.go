package cli

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"strings"

	"github.com/FlanChanXwO/pixiv-mcp-server/internal/download"
	"github.com/FlanChanXwO/pixiv-mcp-server/internal/pixiv"
)

func (a app) runSearch(args []string) error {
	var opts commandOptions
	fs := a.flagSet("search", &opts)
	target := fs.String("search-target", "partial_match_for_tags", "search target")
	sortMode := fs.String("sort", "date_desc", "sort mode")
	duration := fs.String("duration", "", "duration")
	offset := fs.Int("offset", 0, "offset")
	r18 := fs.Bool("r18", false, "append R-18 to the search word")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() == 0 {
		return fmt.Errorf("usage: pixiv search [options] WORD")
	}
	word := strings.Join(fs.Args(), " ")
	if *r18 {
		word += " R-18"
	}
	client, _, err := a.clientAndConfig(opts, false)
	if err != nil {
		return err
	}
	result, err := client.SearchIllust(context.Background(), word, *target, *sortMode, *duration, *offset)
	if err != nil {
		return err
	}
	if opts.jsonOut {
		return a.printJSON(result)
	}
	fmt.Fprintf(a.out, "found %d illustrations for %q\n", len(result.Illusts), word)
	printIllusts(a.out, result.Illusts, *offset, false)
	return nil
}

func (a app) runDetail(args []string) error {
	var opts commandOptions
	fs := a.flagSet("detail", &opts)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: pixiv detail [options] ILLUST_ID")
	}
	id, err := parseInt64Arg(fs.Arg(0), "illust_id")
	if err != nil {
		return err
	}
	client, _, err := a.clientAndConfig(opts, false)
	if err != nil {
		return err
	}
	result, err := client.IllustDetail(context.Background(), id)
	if err != nil {
		return err
	}
	if opts.jsonOut {
		return a.printJSON(result)
	}
	printIllust(a.out, result.Illust, 0, false)
	return nil
}

func (a app) runRanking(args []string) error {
	var opts commandOptions
	fs := a.flagSet("ranking", &opts)
	mode := fs.String("mode", "day", "ranking mode")
	date := fs.String("date", "", "YYYY-MM-DD")
	offset := fs.Int("offset", 0, "offset")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("usage: pixiv ranking [options]")
	}
	client, _, err := a.clientAndConfig(opts, false)
	if err != nil {
		return err
	}
	result, err := client.IllustRanking(context.Background(), *mode, *date, *offset)
	if err != nil {
		return err
	}
	if opts.jsonOut {
		return a.printJSON(result)
	}
	fmt.Fprintf(a.out, "%s ranking\n", *mode)
	printIllusts(a.out, result.Illusts, *offset, true)
	return nil
}

func (a app) runRecommended(args []string) error {
	var opts commandOptions
	fs := a.flagSet("recommended", &opts)
	offset := fs.Int("offset", 0, "offset")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("usage: pixiv recommended [options]")
	}
	client, _, err := a.clientAndConfig(opts, true)
	if err != nil {
		return err
	}
	result, err := client.IllustRecommended(context.Background(), *offset)
	if err != nil {
		return err
	}
	if opts.jsonOut {
		return a.printJSON(result)
	}
	fmt.Fprintln(a.out, "recommended illustrations")
	printIllusts(a.out, result.Illusts, *offset, false)
	return nil
}

func (a app) runDownload(args []string) error {
	var opts commandOptions
	fs := a.flagSet("download", &opts)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() == 0 {
		return fmt.Errorf("usage: pixiv download [options] ILLUST_ID...")
	}
	ids := make([]int64, 0, fs.NArg())
	for _, arg := range fs.Args() {
		id, err := strconv.ParseInt(arg, 10, 64)
		if err != nil || id <= 0 {
			return fmt.Errorf("illust_id %q must be a positive integer", arg)
		}
		ids = append(ids, id)
	}
	client, cfg, err := a.clientAndConfig(opts, true)
	if err != nil {
		return err
	}
	manager := download.NewManager(client, slog.New(slog.NewTextHandler(a.errOut, nil)), cfg.DownloadPath, cfg.FilenameTemplate)
	artworks, err := manager.Download(context.Background(), ids)
	if err != nil {
		return err
	}
	if opts.jsonOut {
		return a.printJSON(artworks)
	}
	for _, artwork := range artworks {
		fmt.Fprintf(a.out, "downloaded %d %q by %s\n", artwork.IllustID, artwork.Title, artwork.Author)
		for _, file := range artwork.Files {
			fmt.Fprintf(a.out, "  %s\n", file.Path)
		}
	}
	return nil
}

func printIllusts(w io.Writer, illusts []pixiv.Illust, offset int, ranked bool) {
	for i, illust := range illusts {
		rank := 0
		if ranked {
			rank = i + 1 + offset
		}
		printIllust(w, illust, rank, true)
	}
}

func printIllust(w io.Writer, illust pixiv.Illust, rank int, compact bool) {
	prefix := ""
	if rank > 0 {
		prefix = fmt.Sprintf("#%d ", rank)
	}
	tags := make([]string, 0, len(illust.Tags))
	for _, tag := range illust.Tags {
		tags = append(tags, tag.Name)
	}
	if compact {
		fmt.Fprintf(w, "%s%d %q by %s bookmarks:%d views:%d tags:%s\n",
			prefix, illust.ID, illust.Title, illust.User.Name, illust.TotalBookmarks, illust.TotalView, strings.Join(tags, ","))
		return
	}
	fmt.Fprintf(w, "id: %d\n", illust.ID)
	fmt.Fprintf(w, "title: %s\n", illust.Title)
	fmt.Fprintf(w, "author: %s (%d)\n", illust.User.Name, illust.User.ID)
	fmt.Fprintf(w, "type: %s\n", illust.Type)
	fmt.Fprintf(w, "page_count: %d\n", illust.PageCount)
	fmt.Fprintf(w, "bookmarks: %d\n", illust.TotalBookmarks)
	fmt.Fprintf(w, "views: %d\n", illust.TotalView)
	fmt.Fprintf(w, "tags: %s\n", strings.Join(tags, ","))
}
