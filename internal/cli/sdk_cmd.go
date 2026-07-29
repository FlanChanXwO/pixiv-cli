package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"

	"github.com/FlanChanXwO/pixiv-cli/internal/application"
	sdk "github.com/FlanChanXwO/pixiv-cli/pixiv"
	"github.com/spf13/cobra"
)

// listOptions 是所有 CLI 列表命令共享的逻辑分页语义。
type listOptions struct {
	limit int
	page  int
}

type listPlan struct {
	limit    int
	skip     int
	oneBatch bool
}

// ndjsonOutputOptions 仅供实体集合命令启用逐行记录输出，避免其他命令误接受该参数。
type ndjsonOutputOptions struct {
	ndjson bool
}

func bindNDJSONFlag(cmd *cobra.Command, options *ndjsonOutputOptions) {
	cmd.Flags().BoolVar(&options.ndjson, "ndjson", false, "print one Pixiv entity record as JSON per line")
}

func bindListFlags(cmd *cobra.Command, options *listOptions) {
	flags := cmd.Flags()
	flags.SetOutput(cmd.ErrOrStderr())
	flags.IntVar(&options.limit, "limit", 0, "maximum results; omitted returns one upstream batch; 0 returns all results")
	flags.IntVar(&options.page, "page", 0, "1-based logical page (requires --limit > 0)")
}

func parseListPlan(cmd *cobra.Command, options listOptions) (listPlan, error) {
	if options.limit < 0 {
		return listPlan{}, errors.New("limit must be zero or a positive integer")
	}
	if cmd.Flags().Changed("page") && options.page <= 0 {
		return listPlan{}, errors.New("page must be a positive integer")
	}
	if options.page > 0 {
		if options.limit <= 0 {
			return listPlan{}, errors.New("--page requires --limit to be a positive integer")
		}
		if options.page-1 > math.MaxInt/options.limit {
			return listPlan{}, errors.New("page and limit overflow the logical result offset")
		}
		return listPlan{limit: options.limit, skip: (options.page - 1) * options.limit}, nil
	}
	return listPlan{limit: options.limit, oneBatch: !cmd.Flags().Changed("limit")}, nil
}

func (a app) sdkRequest(cmd *cobra.Command, options commandOptions) (application.SDKClientRequest, *bool, error) {
	client, err := a.clientRequest(cmd, options, false)
	if err != nil {
		return application.SDKClientRequest{}, nil, err
	}
	return application.SDKClientRequest{
		HTTPSProxyOverride: client.HTTPSProxyOverride,
	}, client.JSONOverride, nil
}

// pageItems 只把 CLI 的分页计划交给 application；cursor 遍历、跳过、限量与止环
// 都由共享 application 引擎负责。
func pageItems[T any](ctx context.Context, plan listPlan, fetch func(context.Context, sdk.Cursor) ([]T, sdk.Cursor, error), consume func([]T) error) error {
	_, err := application.TraversePages(ctx, application.PagePlan{
		Skip:     plan.skip,
		Limit:    plan.limit,
		OneBatch: plan.oneBatch,
	}, fetch, consume)
	return err
}

func encodeNDJSONRecords[T any](encoder *json.Encoder, items []T, mapRecord func(T) (application.Record, error)) error {
	for _, item := range items {
		record, err := mapRecord(item)
		if err != nil {
			return err
		}
		if err := encoder.Encode(record); err != nil {
			return err
		}
	}
	return nil
}

// runIllustListNDJSON 按上游分页立即写出记录；它不能复用 JSON spool，否则下游
// 无法在第二页请求前消费第一页。
func (a app) runIllustListNDJSON(ctx context.Context, plan listPlan, fetch func(context.Context, sdk.Cursor) ([]sdk.Illust, sdk.Cursor, error)) error {
	encoder := json.NewEncoder(a.out)
	return pageItems(ctx, plan, fetch, func(items []sdk.Illust) error {
		return encodeNDJSONRecords(encoder, items, application.RecordFromIllust)
	})
}

// runPooledIllustList 将一个内容读取收敛在账号池的安全重放边界。NDJSON 在每条
// record 成功写出后提交；JSON 在完整临时文档成功交给 stdout 后提交；文本在首批
// 可见作品写出后提交。因而 429 只会在未向用户暴露任何结果时切换本地账号。
func (a app) runPooledIllustList(ctx context.Context, request application.SDKClientRequest, plan listPlan, jsonOut, ndjson bool, heading string, fetch func(application.SDKClient, context.Context, sdk.Cursor) ([]sdk.Illust, sdk.Cursor, error), print func([]sdk.Illust, int) error) error {
	return a.runPooledIllustListWithHeading(ctx, request, plan, jsonOut, ndjson, func() string { return heading }, fetch, print)
}

// runPooledIllustListWithHeading 允许身份由当前池内账号决定的读取在真正输出前再生成标题。
func (a app) runPooledIllustListWithHeading(ctx context.Context, request application.SDKClientRequest, plan listPlan, jsonOut, ndjson bool, heading func() string, fetch func(application.SDKClient, context.Context, sdk.Cursor) ([]sdk.Illust, sdk.Cursor, error), print func([]sdk.Illust, int) error) error {
	return a.runPooledIllustListWithKey(ctx, request, plan, jsonOut, ndjson, "illusts", heading, fetch, print)
}

// runPooledIllustListWithKey 保留少数已有 JSON 兼容键（例如 manga），同时复用完整的
// 账号池提交边界。
func (a app) runPooledIllustListWithKey(ctx context.Context, request application.SDKClientRequest, plan listPlan, jsonOut, ndjson bool, jsonKey string, heading func() string, fetch func(application.SDKClient, context.Context, sdk.Cursor) ([]sdk.Illust, sdk.Cursor, error), print func([]sdk.Illust, int) error) error {
	services := a.services()
	return services.SDK.RunPooledOperation(ctx, request, func(ctx context.Context, client application.SDKClient) (bool, error) {
		committed := false
		boundFetch := func(ctx context.Context, cursor sdk.Cursor) ([]sdk.Illust, sdk.Cursor, error) {
			return fetch(client, ctx, cursor)
		}
		if ndjson {
			encoder := json.NewEncoder(a.out)
			err := pageItems(ctx, plan, boundFetch, func(items []sdk.Illust) error {
				for _, item := range items {
					record, err := application.RecordFromIllust(item)
					if err != nil {
						return err
					}
					// 一旦尝试向 stdout 写入，失败原因即使形似上游 429 也不能
					// 切换账号重放；下游可能已经收到了部分字节。
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
			spool, err := newJSONArraySpool(jsonKey)
			if err != nil {
				return false, err
			}
			defer spool.Close()
			if err := pageItems(ctx, plan, boundFetch, func(items []sdk.Illust) error { return appendJSONArray(spool, items) }); err != nil {
				return false, err
			}
			committed = true
			err = spool.Commit(a.out)
			return committed, err
		}
		position := plan.skip
		headingWritten := false
		err := pageItems(ctx, plan, boundFetch, func(items []sdk.Illust) error {
			if !headingWritten && heading != nil {
				if text := heading(); text != "" {
					committed = true
					if _, err := fmt.Fprintln(a.out, text); err != nil {
						return err
					}
				}
				headingWritten = true
			}
			if len(items) > 0 {
				committed = true
			}
			if err := print(items, position); err != nil {
				return err
			}
			position += len(items)
			committed = true
			return nil
		})
		return committed, err
	})
}

func (a app) runPooledNovelList(ctx context.Context, request application.SDKClientRequest, plan listPlan, jsonOut, ndjson bool, heading string, fetch func(application.SDKClient, context.Context, sdk.Cursor) ([]sdk.Novel, sdk.Cursor, error), print func([]sdk.Novel) error) error {
	services := a.services()
	return services.SDK.RunPooledOperation(ctx, request, func(ctx context.Context, client application.SDKClient) (bool, error) {
		committed := false
		boundFetch := func(ctx context.Context, cursor sdk.Cursor) ([]sdk.Novel, sdk.Cursor, error) {
			return fetch(client, ctx, cursor)
		}
		if ndjson {
			encoder := json.NewEncoder(a.out)
			err := pageItems(ctx, plan, boundFetch, func(items []sdk.Novel) error {
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
			})
			return committed, err
		}
		if jsonOut {
			spool, err := newJSONArraySpool("novels")
			if err != nil {
				return false, err
			}
			defer spool.Close()
			if err := pageItems(ctx, plan, boundFetch, func(items []sdk.Novel) error { return appendJSONArray(spool, items) }); err != nil {
				return false, err
			}
			committed = true
			err = spool.Commit(a.out)
			return committed, err
		}
		headingWritten := false
		err := pageItems(ctx, plan, boundFetch, func(items []sdk.Novel) error {
			if !headingWritten && heading != "" {
				committed = true
				if _, err := fmt.Fprintln(a.out, heading); err != nil {
					return err
				}
				headingWritten = true
			}
			if len(items) > 0 {
				committed = true
			}
			if err := print(items); err != nil {
				return err
			}
			committed = true
			return nil
		})
		return committed, err
	})
}

// heading 在首批实际输出前才求值，使取当前账号身份等前置读取也位于可安全重放的边界内。
func (a app) runPooledUserList(ctx context.Context, request application.SDKClientRequest, plan listPlan, jsonOut, ndjson bool, heading func() string, fetch func(application.SDKClient, context.Context, sdk.Cursor) ([]sdk.UserPreview, sdk.Cursor, error), print func([]sdk.UserPreview) error) error {
	services := a.services()
	return services.SDK.RunPooledOperation(ctx, request, func(ctx context.Context, client application.SDKClient) (bool, error) {
		committed := false
		boundFetch := func(ctx context.Context, cursor sdk.Cursor) ([]sdk.UserPreview, sdk.Cursor, error) {
			return fetch(client, ctx, cursor)
		}
		if ndjson {
			encoder := json.NewEncoder(a.out)
			err := pageItems(ctx, plan, boundFetch, func(items []sdk.UserPreview) error {
				for _, item := range items {
					record, err := application.RecordFromUserPreview(item)
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
			if err := pageItems(ctx, plan, boundFetch, func(items []sdk.UserPreview) error { return appendJSONArray(spool, items) }); err != nil {
				return false, err
			}
			committed = true
			err = spool.Commit(a.out)
			return committed, err
		}
		headingWritten := false
		err := pageItems(ctx, plan, boundFetch, func(items []sdk.UserPreview) error {
			if !headingWritten && heading != nil {
				if text := heading(); text != "" {
					committed = true
					if _, err := fmt.Fprintln(a.out, text); err != nil {
						return err
					}
				}
				headingWritten = true
			}
			if len(items) > 0 {
				committed = true
			}
			if err := print(items); err != nil {
				return err
			}
			committed = true
			return nil
		})
		return committed, err
	})
}

func (a app) runIllustList(ctx context.Context, plan listPlan, jsonOut bool, fetch func(context.Context, sdk.Cursor) ([]sdk.Illust, sdk.Cursor, error), print func([]sdk.Illust, int) error) error {
	if jsonOut {
		spool, err := newJSONArraySpool("illusts")
		if err != nil {
			return err
		}
		defer spool.Close()
		if err := pageItems(ctx, plan, fetch, func(items []sdk.Illust) error { return appendJSONArray(spool, items) }); err != nil {
			return err
		}
		return spool.Commit(a.out)
	}
	position := plan.skip
	return pageItems(ctx, plan, fetch, func(items []sdk.Illust) error {
		if err := print(items, position); err != nil {
			return err
		}
		position += len(items)
		return nil
	})
}

func (a app) runUserList(ctx context.Context, plan listPlan, jsonOut bool, fetch func(context.Context, sdk.Cursor) ([]sdk.UserPreview, sdk.Cursor, error), print func([]sdk.UserPreview) error) error {
	if jsonOut {
		spool, err := newJSONArraySpool("user_previews")
		if err != nil {
			return err
		}
		defer spool.Close()
		if err := pageItems(ctx, plan, fetch, func(items []sdk.UserPreview) error { return appendJSONArray(spool, items) }); err != nil {
			return err
		}
		return spool.Commit(a.out)
	}
	return pageItems(ctx, plan, fetch, print)
}

// printUserPreviews 保留文本列表的一行一个用户格式，并将 stdout 失败交给调用方。
func printUserPreviews(w io.Writer, users []sdk.UserPreview) error {
	for _, item := range users {
		if _, err := fmt.Fprintf(w, "%d %s\n", item.User.ID, item.User.Name); err != nil {
			return err
		}
	}
	return nil
}

func (a app) runUserListNDJSON(ctx context.Context, plan listPlan, fetch func(context.Context, sdk.Cursor) ([]sdk.UserPreview, sdk.Cursor, error)) error {
	encoder := json.NewEncoder(a.out)
	return pageItems(ctx, plan, fetch, func(items []sdk.UserPreview) error {
		return encodeNDJSONRecords(encoder, items, application.RecordFromUserPreview)
	})
}

// jsonArraySpool 将已确认成功的每个元素逐个写入私有临时文件。网络或编码失败时
// stdout 完全不写；成功后才把一个完整 JSON document 原子性地转交给调用方 writer。
type jsonArraySpool struct {
	file  *os.File
	first bool
	key   string
}

func newJSONArraySpool(key string) (*jsonArraySpool, error) {
	file, err := os.CreateTemp("", "pixiv-cli-json-*.tmp")
	if err != nil {
		return nil, err
	}
	spool := &jsonArraySpool{file: file, first: true, key: key}
	if _, err := fmt.Fprintf(file, "{\n  %q: [", key); err != nil {
		_ = file.Close()
		_ = os.Remove(file.Name())
		return nil, err
	}
	return spool, nil
}

func appendJSONArray[T any](s *jsonArraySpool, items []T) error {
	for _, item := range items {
		if s.first {
			s.first = false
		} else if _, err := io.WriteString(s.file, ","); err != nil {
			return err
		}
		encoded, err := json.MarshalIndent(item, "    ", "  ")
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(s.file, "\n    %s", encoded); err != nil {
			return err
		}
	}
	return nil
}

func (s *jsonArraySpool) Commit(out io.Writer) error {
	return s.commit(out, "", "")
}

// CommitWithStringField 在数组完成后追加稳定的顶层字符串字段，仍保持 stdout 的
// 原子提交语义，供 source 这类列表元数据使用。
func (s *jsonArraySpool) CommitWithStringField(out io.Writer, name, value string) error {
	return s.commit(out, name, value)
}

func (s *jsonArraySpool) commit(out io.Writer, extraName, extraValue string) error {
	if _, err := io.WriteString(s.file, "\n  ]"); err != nil {
		return err
	}
	if extraName != "" {
		encodedName, err := json.Marshal(extraName)
		if err != nil {
			return err
		}
		encodedValue, err := json.Marshal(extraValue)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(s.file, ",\n  %s: %s", encodedName, encodedValue); err != nil {
			return err
		}
	}
	if _, err := io.WriteString(s.file, "\n}\n"); err != nil {
		return err
	}
	if _, err := s.file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	_, err := io.Copy(out, s.file)
	return err
}

func (s *jsonArraySpool) Close() {
	if s == nil || s.file == nil {
		return
	}
	name := s.file.Name()
	_ = s.file.Close()
	_ = os.Remove(name)
}
