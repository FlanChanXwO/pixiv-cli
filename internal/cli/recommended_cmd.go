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
	if err := applyIllustFilter(&plan, opts.filter); err != nil {
		return err
	}
	if plan.filter != nil && (kind == "novel" || kind == "user") {
		return newUsageError(fmt.Errorf("--filter is only available for illustration recommendations"))
	}
	services := a.services()
	request, jsonOverride, err := a.sdkRequest(cmd, opts.commandOptions)
	if err != nil {
		return err
	}
	if opts.ndjson && cmd.Flags().Changed("json") {
		return newUsageError(fmt.Errorf("--ndjson cannot be used with --json"))
	}
	jsonOut := false
	if !opts.ndjson {
		jsonOut, err = services.SDK.JSONOut(jsonOverride)
		if err != nil {
			return err
		}
	}
	opts.ndjson = a.shouldAutoNDJSON(cmd, opts.ndjson, jsonOut)
	if kind == "all" {
		if opts.ndjson {
			return services.SDK.RunPooledOperation(cmd.Context(), request, func(ctx context.Context, client application.SDKClient) (bool, error) {
				return a.runRecommendedAllNDJSON(ctx, client, plan)
			})
		}
		return services.SDK.RunPooledOperation(cmd.Context(), request, func(ctx context.Context, client application.SDKClient) (bool, error) {
			return a.runRecommendedAll(ctx, client, plan, jsonOut)
		})
	}
	return a.runRecommendedOnePooled(cmd.Context(), request, plan, jsonOut, opts.ndjson, kind)
}

func validRecommendationKind(kind string) bool {
	return kind == "all" || kind == "illust" || kind == "manga" || kind == "novel" || kind == "user"
}

func (a app) runRecommendedOnePooled(ctx context.Context, request application.SDKClientRequest, plan listPlan, jsonOut, ndjson bool, kind string) error {
	if kind == "illust" || kind == "manga" {
		jsonKey := "illusts"
		if kind == "manga" {
			jsonKey = "manga"
		}
		fetch := func(client application.SDKClient, ctx context.Context, cursor sdk.Cursor) ([]sdk.Illust, sdk.Cursor, error) {
			var result *sdk.IllustListResult
			var err error
			if kind == "illust" {
				result, err = client.IllustRecommended(ctx, sdk.IllustRecommendedRequest{Cursor: cursor})
			} else {
				result, err = client.MangaRecommended(ctx, sdk.IllustRecommendedRequest{Cursor: cursor})
			}
			if err != nil {
				return nil, "", err
			}
			return result.Illusts, result.NextCursor, nil
		}
		return a.runPooledIllustListWithKey(ctx, request, plan, jsonOut, ndjson, jsonKey, func() string {
			return fmt.Sprintf("recommended %s", kind)
		}, fetch, func(items []sdk.Illust, start int) error { return printIllusts(a.out, items, start, false) })
	}
	if kind == "novel" {
		fetch := func(client application.SDKClient, ctx context.Context, cursor sdk.Cursor) ([]sdk.Novel, sdk.Cursor, error) {
			result, err := client.NovelRecommended(ctx, sdk.NovelRecommendedRequest{Cursor: cursor})
			if err != nil {
				return nil, "", err
			}
			return result.Novels, result.NextCursor, nil
		}
		return a.runPooledNovelList(ctx, request, plan, jsonOut, ndjson, "recommended novels", fetch, func(items []sdk.Novel) error { return printNovels(a.out, items) })
	}
	fetch := func(client application.SDKClient, ctx context.Context, cursor sdk.Cursor) ([]sdk.RecommendedUserPreview, sdk.Cursor, error) {
		result, err := client.UserRecommended(ctx, sdk.UserRecommendedRequest{Cursor: cursor})
		if err != nil {
			return nil, "", err
		}
		return result.UserPreviews, result.NextCursor, nil
	}
	return a.runPooledRecommendedUserList(ctx, request, plan, jsonOut, ndjson, fetch)
}

func (a app) runPooledRecommendedUserList(ctx context.Context, request application.SDKClientRequest, plan listPlan, jsonOut, ndjson bool, fetch func(application.SDKClient, context.Context, sdk.Cursor) ([]sdk.RecommendedUserPreview, sdk.Cursor, error)) error {
	services := a.services()
	return services.SDK.RunPooledOperation(ctx, request, func(ctx context.Context, client application.SDKClient) (bool, error) {
		boundFetch := func(ctx context.Context, cursor sdk.Cursor) ([]sdk.RecommendedUserPreview, sdk.Cursor, error) {
			return fetch(client, ctx, cursor)
		}
		committed := false
		if ndjson {
			encoder := json.NewEncoder(a.out)
			err := pageItems(ctx, plan, boundFetch, func(items []sdk.RecommendedUserPreview) error {
				for _, item := range items {
					record, err := application.RecordFromRecommendedUserPreview(item)
					if err != nil {
						return err
					}
					committed = true
					if err := encoder.Encode(record); err != nil {
						return err
					}
				}
				return nil
			})
			return committed, err
		}
		if jsonOut {
			spool, err := newJSONArraySpool("user_previews")
			if err != nil {
				return false, err
			}
			defer spool.Close()
			if err := pageItems(ctx, plan, boundFetch, func(items []sdk.RecommendedUserPreview) error { return appendJSONArray(spool, items) }); err != nil {
				return false, err
			}
			committed = true
			err = spool.Commit(a.out)
			return committed, err
		}
		headingWritten := false
		err := pageItems(ctx, plan, boundFetch, func(items []sdk.RecommendedUserPreview) error {
			if !headingWritten {
				committed = true
				if _, err := fmt.Fprintln(a.out, "recommended users"); err != nil {
					return err
				}
				headingWritten = true
			}
			if len(items) > 0 {
				committed = true
			}
			if err := printRecommendedUsers(a.out, items); err != nil {
				return err
			}
			return nil
		})
		return committed, err
	})
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
			if _, err := fmt.Fprintf(a.out, "recommended %s\n", kind); err != nil {
				return err
			}
		}
		return a.runIllustList(ctx, plan, jsonOut, fetch, func(items []sdk.Illust, start int) error { return printIllusts(a.out, items, start, false) })
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
	if _, err := fmt.Fprintln(a.out, "recommended novels"); err != nil {
		return err
	}
	return pageItems(ctx, plan, fetch, func(items []sdk.Novel) error { return printNovels(a.out, items) })
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
	if _, err := fmt.Fprintln(a.out, "recommended users"); err != nil {
		return err
	}
	return pageItems(ctx, plan, fetch, func(items []sdk.RecommendedUserPreview) error { return printRecommendedUsers(a.out, items) })
}

func (a app) runRecommendedAll(ctx context.Context, client application.SDKClient, plan listPlan, jsonOut bool) (bool, error) {
	s, err := newRecommendationSpool(jsonOut)
	if err != nil {
		return false, err
	}
	defer s.Close()
	if err := s.section("illusts"); err != nil {
		return false, err
	}
	if !jsonOut {
		if _, err := fmt.Fprintln(s.file, "recommended illustrations"); err != nil {
			return false, err
		}
	}
	if err := pageItems(ctx, plan, func(ctx context.Context, c sdk.Cursor) ([]sdk.Illust, sdk.Cursor, error) {
		r, e := client.IllustRecommended(ctx, sdk.IllustRecommendedRequest{Cursor: c})
		if e != nil {
			return nil, "", e
		}
		items, filterErr := filterIllustBatch(plan.filter, r.Illusts)
		return items, r.NextCursor, filterErr
	}, s.illusts); err != nil {
		return false, err
	}
	if err := s.section("manga"); err != nil {
		return false, err
	}
	if !jsonOut {
		if _, err := fmt.Fprintln(s.file, "recommended manga"); err != nil {
			return false, err
		}
	}
	if err := pageItems(ctx, plan, func(ctx context.Context, c sdk.Cursor) ([]sdk.Illust, sdk.Cursor, error) {
		r, e := client.MangaRecommended(ctx, sdk.IllustRecommendedRequest{Cursor: c})
		if e != nil {
			return nil, "", e
		}
		items, filterErr := filterIllustBatch(plan.filter, r.Illusts)
		return items, r.NextCursor, filterErr
	}, s.illusts); err != nil {
		return false, err
	}
	if plan.filter != nil {
		// 表达式的输入域是 Illust；mixed recommendations 里其余实体没有这些字段，
		// 因此明确不输出它们，而不是伪造零值再尝试匹配。
		return true, s.Commit(a.out)
	}
	if err := s.section("novels"); err != nil {
		return false, err
	}
	if !jsonOut {
		if _, err := fmt.Fprintln(s.file, "recommended novels"); err != nil {
			return false, err
		}
	}
	if err := pageItems(ctx, plan, func(ctx context.Context, c sdk.Cursor) ([]sdk.Novel, sdk.Cursor, error) {
		r, e := client.NovelRecommended(ctx, sdk.NovelRecommendedRequest{Cursor: c})
		if e != nil {
			return nil, "", e
		}
		return r.Novels, r.NextCursor, nil
	}, s.novels); err != nil {
		return false, err
	}
	if err := s.section("user_previews"); err != nil {
		return false, err
	}
	if !jsonOut {
		if _, err := fmt.Fprintln(s.file, "recommended users"); err != nil {
			return false, err
		}
	}
	if err := pageItems(ctx, plan, func(ctx context.Context, c sdk.Cursor) ([]sdk.RecommendedUserPreview, sdk.Cursor, error) {
		r, e := client.UserRecommended(ctx, sdk.UserRecommendedRequest{Cursor: c})
		if e != nil {
			return nil, "", e
		}
		return r.UserPreviews, r.NextCursor, nil
	}, s.users); err != nil {
		return false, err
	}
	// 最终 io.Copy 可能已交付部分 stdout；在尝试前关闭账号池重放窗口。
	return true, s.Commit(a.out)
}

// runRecommendedAllNDJSON 保留 all 的既有类别顺序。每写出一条记录即标记提交，
// 因此账号池只会在任何下游可见输出之前重放 429。
func (a app) runRecommendedAllNDJSON(ctx context.Context, client application.SDKClient, plan listPlan) (bool, error) {
	encoder := json.NewEncoder(a.out)
	committed := false
	writeIllusts := func(items []sdk.Illust) error {
		for _, item := range items {
			record, err := application.RecordFromIllust(item)
			if err != nil {
				return err
			}
			committed = true
			if err := encoder.Encode(record); err != nil {
				return err
			}
		}
		return nil
	}
	if err := pageItems(ctx, plan, func(ctx context.Context, cursor sdk.Cursor) ([]sdk.Illust, sdk.Cursor, error) {
		result, err := client.IllustRecommended(ctx, sdk.IllustRecommendedRequest{Cursor: cursor})
		if err != nil {
			return nil, "", err
		}
		items, filterErr := filterIllustBatch(plan.filter, result.Illusts)
		return items, result.NextCursor, filterErr
	}, writeIllusts); err != nil {
		return committed, err
	}
	if err := pageItems(ctx, plan, func(ctx context.Context, cursor sdk.Cursor) ([]sdk.Illust, sdk.Cursor, error) {
		result, err := client.MangaRecommended(ctx, sdk.IllustRecommendedRequest{Cursor: cursor})
		if err != nil {
			return nil, "", err
		}
		items, filterErr := filterIllustBatch(plan.filter, result.Illusts)
		return items, result.NextCursor, filterErr
	}, writeIllusts); err != nil {
		return committed, err
	}
	if plan.filter != nil {
		return committed, nil
	}
	if err := pageItems(ctx, plan, func(ctx context.Context, cursor sdk.Cursor) ([]sdk.Novel, sdk.Cursor, error) {
		result, err := client.NovelRecommended(ctx, sdk.NovelRecommendedRequest{Cursor: cursor})
		if err != nil {
			return nil, "", err
		}
		return result.Novels, result.NextCursor, nil
	}, func(items []sdk.Novel) error {
		for _, item := range items {
			record, err := application.RecordFromNovel(item)
			if err != nil {
				return err
			}
			committed = true
			if err := encoder.Encode(record); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return committed, err
	}
	err := pageItems(ctx, plan, func(ctx context.Context, cursor sdk.Cursor) ([]sdk.RecommendedUserPreview, sdk.Cursor, error) {
		result, err := client.UserRecommended(ctx, sdk.UserRecommendedRequest{Cursor: cursor})
		if err != nil {
			return nil, "", err
		}
		return result.UserPreviews, result.NextCursor, nil
	}, func(items []sdk.RecommendedUserPreview) error {
		for _, item := range items {
			record, err := application.RecordFromRecommendedUserPreview(item)
			if err != nil {
				return err
			}
			committed = true
			if err := encoder.Encode(record); err != nil {
				return err
			}
		}
		return nil
	})
	return committed, err
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
			return printIllusts(s.file, []sdk.Illust{v}, 0, false)
		case sdk.Novel:
			return printNovels(s.file, []sdk.Novel{v})
		case sdk.RecommendedUserPreview:
			return printRecommendedUsers(s.file, []sdk.RecommendedUserPreview{v})
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
func printNovels(w io.Writer, novels []sdk.Novel) error {
	for _, n := range novels {
		if _, err := fmt.Fprintf(w, "%d %s — %s\n", n.ID, n.Title, n.User.Name); err != nil {
			return err
		}
	}
	return nil
}
func printRecommendedUsers(w io.Writer, users []sdk.RecommendedUserPreview) error {
	for _, u := range users {
		if _, err := fmt.Fprintf(w, "%d %s\n", u.User.ID, u.User.Name); err != nil {
			return err
		}
	}
	return nil
}
