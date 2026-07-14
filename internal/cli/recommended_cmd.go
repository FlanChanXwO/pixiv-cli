package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/FlanChanXwO/pixiv-cli/internal/application"
	sdk "github.com/FlanChanXwO/pixiv-cli/pixiv"
	"github.com/spf13/cobra"
)

func (a app) runRecommended(cmd *cobra.Command, kind string, opts recommendedOptions) error {
	if !validRecommendationKind(kind) {
		return fmt.Errorf("recommendation kind must be one of: all, illust, manga, novel, user")
	}
	plan, err := parseListPlan(cmd, opts.listOptions)
	if err != nil {
		return err
	}
	services := a.services()
	request, jsonOverride, err := a.sdkRequest(cmd, opts.commandOptions)
	if err != nil {
		return err
	}
	jsonOut, err := services.SDK.JSONOut(jsonOverride)
	if err != nil {
		return err
	}
	client, err := services.SDK.OpenOperation(cmd.Context(), request)
	if err != nil {
		return err
	}
	if kind == "all" {
		return a.runRecommendedAll(cmd.Context(), client, plan, jsonOut)
	}
	return a.runRecommendedOne(cmd.Context(), client, plan, jsonOut, kind)
}

func validRecommendationKind(kind string) bool {
	return kind == "all" || kind == "illust" || kind == "manga" || kind == "novel" || kind == "user"
}

func (a app) runRecommendedOne(ctx context.Context, client application.SDKClient, plan listPlan, jsonOut bool, kind string) error {
	if kind == "illust" || kind == "manga" {
		fetch := func(ctx context.Context, cursor sdk.Cursor) ([]sdk.Illust, sdk.Cursor, error) {
			var r *sdk.IllustListResult
			var err error
			if kind == "illust" {
				r, err = client.IllustRecommended(ctx, sdk.IllustRecommendedRequest{Cursor: cursor})
			} else {
				r, err = client.MangaRecommended(ctx, sdk.IllustRecommendedRequest{Cursor: cursor})
			}
			if err != nil {
				return nil, "", err
			}
			return r.Illusts, r.NextCursor, nil
		}
		if kind == "manga" && jsonOut {
			spool, err := newJSONArraySpool("manga")
			if err != nil {
				return err
			}
			defer spool.Close()
			if err := pageItems(ctx, plan, fetch, func(items []sdk.Illust) error { return appendJSONArray(spool, items) }); err != nil {
				return err
			}
			return spool.Commit(a.out)
		}
		if !jsonOut {
			fmt.Fprintf(a.out, "recommended %s\n", kind)
		}
		return a.runIllustList(ctx, plan, jsonOut, fetch, func(items []sdk.Illust, start int) { printIllusts(a.out, items, start, false) })
	}
	if kind == "novel" {
		return a.runRecommendedNovels(ctx, client, plan, jsonOut)
	}
	return a.runRecommendedUsers(ctx, client, plan, jsonOut)
}

func (a app) runRecommendedNovels(ctx context.Context, client application.SDKClient, plan listPlan, jsonOut bool) error {
	fetch := func(ctx context.Context, cursor sdk.Cursor) ([]sdk.Novel, sdk.Cursor, error) {
		r, err := client.NovelRecommended(ctx, sdk.NovelRecommendedRequest{Cursor: cursor})
		if err != nil {
			return nil, "", err
		}
		return r.Novels, r.NextCursor, nil
	}
	if jsonOut {
		s, err := newJSONArraySpool("novels")
		if err != nil {
			return err
		}
		defer s.Close()
		if err := pageItems(ctx, plan, fetch, func(items []sdk.Novel) error { return appendJSONArray(s, items) }); err != nil {
			return err
		}
		return s.Commit(a.out)
	}
	fmt.Fprintln(a.out, "recommended novels")
	return pageItems(ctx, plan, fetch, func(items []sdk.Novel) error { printNovels(a.out, items); return nil })
}

func (a app) runRecommendedUsers(ctx context.Context, client application.SDKClient, plan listPlan, jsonOut bool) error {
	fetch := func(ctx context.Context, cursor sdk.Cursor) ([]sdk.RecommendedUserPreview, sdk.Cursor, error) {
		r, err := client.UserRecommended(ctx, sdk.UserRecommendedRequest{Cursor: cursor})
		if err != nil {
			return nil, "", err
		}
		return r.UserPreviews, r.NextCursor, nil
	}
	if jsonOut {
		s, err := newJSONArraySpool("user_previews")
		if err != nil {
			return err
		}
		defer s.Close()
		if err := pageItems(ctx, plan, fetch, func(items []sdk.RecommendedUserPreview) error { return appendJSONArray(s, items) }); err != nil {
			return err
		}
		return s.Commit(a.out)
	}
	fmt.Fprintln(a.out, "recommended users")
	return pageItems(ctx, plan, fetch, func(items []sdk.RecommendedUserPreview) error { printRecommendedUsers(a.out, items); return nil })
}

func (a app) runRecommendedAll(ctx context.Context, client application.SDKClient, plan listPlan, jsonOut bool) error {
	s, err := newRecommendationSpool(jsonOut)
	if err != nil {
		return err
	}
	defer s.Close()
	if err := s.section("illusts"); err != nil {
		return err
	}
	if !jsonOut {
		fmt.Fprintln(s.file, "recommended illustrations")
	}
	if err := pageItems(ctx, plan, func(ctx context.Context, c sdk.Cursor) ([]sdk.Illust, sdk.Cursor, error) {
		r, e := client.IllustRecommended(ctx, sdk.IllustRecommendedRequest{Cursor: c})
		if e != nil {
			return nil, "", e
		}
		return r.Illusts, r.NextCursor, nil
	}, s.illusts); err != nil {
		return err
	}
	if err := s.section("manga"); err != nil {
		return err
	}
	if !jsonOut {
		fmt.Fprintln(s.file, "recommended manga")
	}
	if err := pageItems(ctx, plan, func(ctx context.Context, c sdk.Cursor) ([]sdk.Illust, sdk.Cursor, error) {
		r, e := client.MangaRecommended(ctx, sdk.IllustRecommendedRequest{Cursor: c})
		if e != nil {
			return nil, "", e
		}
		return r.Illusts, r.NextCursor, nil
	}, s.illusts); err != nil {
		return err
	}
	if err := s.section("novels"); err != nil {
		return err
	}
	if !jsonOut {
		fmt.Fprintln(s.file, "recommended novels")
	}
	if err := pageItems(ctx, plan, func(ctx context.Context, c sdk.Cursor) ([]sdk.Novel, sdk.Cursor, error) {
		r, e := client.NovelRecommended(ctx, sdk.NovelRecommendedRequest{Cursor: c})
		if e != nil {
			return nil, "", e
		}
		return r.Novels, r.NextCursor, nil
	}, s.novels); err != nil {
		return err
	}
	if err := s.section("user_previews"); err != nil {
		return err
	}
	if !jsonOut {
		fmt.Fprintln(s.file, "recommended users")
	}
	if err := pageItems(ctx, plan, func(ctx context.Context, c sdk.Cursor) ([]sdk.RecommendedUserPreview, sdk.Cursor, error) {
		r, e := client.UserRecommended(ctx, sdk.UserRecommendedRequest{Cursor: c})
		if e != nil {
			return nil, "", e
		}
		return r.UserPreviews, r.NextCursor, nil
	}, s.users); err != nil {
		return err
	}
	return s.Commit(a.out)
}

type recommendationSpool struct {
	file                             *os.File
	jsonOut, firstSection, firstItem bool
}

var writeRecommendationSpoolHeader = io.WriteString

func newRecommendationSpool(jsonOut bool) (*recommendationSpool, error) {
	f, e := os.CreateTemp("", "pixiv-cli-recommended-*.tmp")
	if e != nil {
		return nil, e
	}
	s := &recommendationSpool{file: f, jsonOut: jsonOut, firstSection: true, firstItem: true}
	if jsonOut {
		_, e = writeRecommendationSpoolHeader(f, "{")
		if e != nil {
			name := f.Name()
			_ = f.Close()
			_ = os.Remove(name)
			return nil, e
		}
	}
	return s, e
}
func (s *recommendationSpool) section(key string) error {
	s.firstItem = true
	if !s.jsonOut {
		return nil
	}
	if !s.firstSection {
		if _, e := io.WriteString(s.file, "\n  ],"); e != nil {
			return e
		}
	}
	s.firstSection = false
	_, e := fmt.Fprintf(s.file, "\n  %q: [", key)
	return e
}
func (s *recommendationSpool) write(value any) error {
	if !s.jsonOut {
		switch v := value.(type) {
		case sdk.Illust:
			printIllusts(s.file, []sdk.Illust{v}, 0, false)
		case sdk.Novel:
			printNovels(s.file, []sdk.Novel{v})
		case sdk.RecommendedUserPreview:
			printRecommendedUsers(s.file, []sdk.RecommendedUserPreview{v})
		}
		return nil
	}
	if !s.firstItem {
		if _, e := io.WriteString(s.file, ","); e != nil {
			return e
		}
	}
	s.firstItem = false
	b, e := json.Marshal(value)
	if e != nil {
		return e
	}
	_, e = fmt.Fprintf(s.file, "\n    %s", b)
	return e
}
func (s *recommendationSpool) illusts(items []sdk.Illust) error {
	for _, v := range items {
		if e := s.write(v); e != nil {
			return e
		}
	}
	return nil
}
func (s *recommendationSpool) novels(items []sdk.Novel) error {
	for _, v := range items {
		if e := s.write(v); e != nil {
			return e
		}
	}
	return nil
}
func (s *recommendationSpool) users(items []sdk.RecommendedUserPreview) error {
	for _, v := range items {
		if e := s.write(v); e != nil {
			return e
		}
	}
	return nil
}
func (s *recommendationSpool) Commit(out io.Writer) error {
	if s.jsonOut {
		if _, e := io.WriteString(s.file, "\n  ]\n}\n"); e != nil {
			return e
		}
	}
	if _, e := s.file.Seek(0, io.SeekStart); e != nil {
		return e
	}
	_, e := io.Copy(out, s.file)
	return e
}
func (s *recommendationSpool) Close() {
	if s == nil || s.file == nil {
		return
	}
	n := s.file.Name()
	_ = s.file.Close()
	_ = os.Remove(n)
}
func printNovels(w io.Writer, novels []sdk.Novel) {
	for _, n := range novels {
		fmt.Fprintf(w, "%d %s — %s\n", n.ID, n.Title, n.User.Name)
	}
}
func printRecommendedUsers(w io.Writer, users []sdk.RecommendedUserPreview) {
	for _, u := range users {
		fmt.Fprintf(w, "%d %s\n", u.User.ID, u.User.Name)
	}
}
