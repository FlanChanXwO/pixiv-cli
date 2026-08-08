package pixiv

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"

	paginationapp "github.com/FlanChanXwO/pixiv-cli/internal/application/pagination"
	pixivapp "github.com/FlanChanXwO/pixiv-cli/internal/application/pixiv"
	recordpkg "github.com/FlanChanXwO/pixiv-cli/internal/record"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/spf13/cobra"
	"golang.org/x/term"
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

func (a controller) shouldAutoNDJSON(cmd *cobra.Command, ndjson, jsonOut bool) bool {
	if ndjson || jsonOut || cmd.Flags().Changed("json") {
		return ndjson
	}
	file, ok := a.out.(interface{ Fd() uintptr })
	return ok && !term.IsTerminal(int(file.Fd()))
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

func (a controller) sdkRequest(cmd *cobra.Command, options commandOptions) (pixivapp.SDKClientRequest, *bool, error) {
	client, err := a.clientRequest(cmd, options, false)
	if err != nil {
		return pixivapp.SDKClientRequest{}, nil, err
	}
	return pixivapp.SDKClientRequest{
		HTTPSProxyOverride:      client.HTTPSProxyOverride,
		RequestIntervalOverride: client.RequestIntervalOverride,
	}, client.JSONOverride, nil
}

// pageItems 只把 CLI 的分页计划交给 application；cursor 遍历、跳过、限量与止环
// 都由共享 application 引擎负责。
func pageItems[T any](ctx context.Context, plan listPlan, fetch func(context.Context, sdk.Cursor) ([]T, sdk.Cursor, error), consume func([]T) error) error {
	_, err := paginationapp.TraversePages(ctx, paginationapp.PagePlan{
		Skip:     plan.skip,
		Limit:    plan.limit,
		OneBatch: plan.oneBatch,
	}, fetch, consume)
	return err
}

// runPooledIllustList 将一个内容读取收敛在账号池的安全重放边界。NDJSON 在每条
// record 成功写出后提交；JSON 在完整临时文档成功交给 stdout 后提交；文本在首批
// 可见作品写出后提交。因而 429 只会在未向用户暴露任何结果时切换本地账号。
func (a controller) runPooledIllustList(ctx context.Context, request pixivapp.SDKClientRequest, plan listPlan, jsonOut, ndjson bool, heading string, fetch func(pixivapp.ClientSet, context.Context, sdk.Cursor) ([]pixiv.Artwork, sdk.Cursor, error), print func([]pixiv.Artwork, int) error) error {
	return a.runPooledIllustListWithHeading(ctx, request, plan, jsonOut, ndjson, func() string { return heading }, fetch, print)
}

// runPooledIllustListWithHeading 允许身份由当前池内账号决定的读取在真正输出前再生成标题。
func (a controller) runPooledIllustListWithHeading(ctx context.Context, request pixivapp.SDKClientRequest, plan listPlan, jsonOut, ndjson bool, heading func() string, fetch func(pixivapp.ClientSet, context.Context, sdk.Cursor) ([]pixiv.Artwork, sdk.Cursor, error), print func([]pixiv.Artwork, int) error) error {
	return a.runPooledIllustListWithKey(ctx, request, plan, jsonOut, ndjson, "illusts", heading, fetch, print)
}

// runPooledIllustListWithKey 保留少数已有 JSON 兼容键（例如 manga），同时复用完整的
// 账号池提交边界。
func (a controller) runPooledIllustListWithKey(ctx context.Context, request pixivapp.SDKClientRequest, plan listPlan, jsonOut, ndjson bool, jsonKey string, heading func() string, fetch func(pixivapp.ClientSet, context.Context, sdk.Cursor) ([]pixiv.Artwork, sdk.Cursor, error), print func([]pixiv.Artwork, int) error) error {
	services := a.services()
	return services.SDK.RunPooledOperation(ctx, request, func(ctx context.Context, client pixivapp.ClientSet) (bool, error) {
		committed := false
		boundFetch := func(ctx context.Context, cursor sdk.Cursor) ([]pixiv.Artwork, sdk.Cursor, error) {
			return fetch(client, ctx, cursor)
		}
		if ndjson {
			encoder := json.NewEncoder(a.out)
			err := pageItems(ctx, plan, boundFetch, func(items []pixiv.Artwork) error {
				for _, item := range items {
					record, err := recordpkg.RecordFromArtwork(item)
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
			if err := pageItems(ctx, plan, boundFetch, func(items []pixiv.Artwork) error { return appendJSONArray(spool, items) }); err != nil {
				return false, err
			}
			committed = true
			err = spool.Commit(a.out)
			return committed, err
		}
		position := plan.skip
		headingWritten := false
		err := pageItems(ctx, plan, boundFetch, func(items []pixiv.Artwork) error {
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

func (a controller) runPooledNovelList(ctx context.Context, request pixivapp.SDKClientRequest, plan listPlan, jsonOut, ndjson bool, heading string, fetch func(pixivapp.ClientSet, context.Context, sdk.Cursor) ([]pixiv.Novel, sdk.Cursor, error), print func([]pixiv.Novel) error) error {
	services := a.services()
	return services.SDK.RunPooledOperation(ctx, request, func(ctx context.Context, client pixivapp.ClientSet) (bool, error) {
		committed := false
		boundFetch := func(ctx context.Context, cursor sdk.Cursor) ([]pixiv.Novel, sdk.Cursor, error) {
			return fetch(client, ctx, cursor)
		}
		if ndjson {
			encoder := json.NewEncoder(a.out)
			err := pageItems(ctx, plan, boundFetch, func(items []pixiv.Novel) error {
				for _, item := range items {
					record, err := recordpkg.RecordFromNovel(item)
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
			if err := pageItems(ctx, plan, boundFetch, func(items []pixiv.Novel) error { return appendJSONArray(spool, items) }); err != nil {
				return false, err
			}
			committed = true
			err = spool.Commit(a.out)
			return committed, err
		}
		headingWritten := false
		err := pageItems(ctx, plan, boundFetch, func(items []pixiv.Novel) error {
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
func (a controller) runPooledUserList(ctx context.Context, request pixivapp.SDKClientRequest, plan listPlan, jsonOut, ndjson bool, heading func() string, fetch func(pixivapp.ClientSet, context.Context, sdk.Cursor) ([]pixiv.UserPreview, sdk.Cursor, error), print func([]pixiv.UserPreview) error) error {
	services := a.services()
	return services.SDK.RunPooledOperation(ctx, request, func(ctx context.Context, client pixivapp.ClientSet) (bool, error) {
		committed := false
		boundFetch := func(ctx context.Context, cursor sdk.Cursor) ([]pixiv.UserPreview, sdk.Cursor, error) {
			return fetch(client, ctx, cursor)
		}
		if ndjson {
			encoder := json.NewEncoder(a.out)
			err := pageItems(ctx, plan, boundFetch, func(items []pixiv.UserPreview) error {
				for _, item := range items {
					record, err := recordpkg.RecordFromUserPreview(item)
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
			if err := pageItems(ctx, plan, boundFetch, func(items []pixiv.UserPreview) error { return appendJSONArray(spool, items) }); err != nil {
				return false, err
			}
			committed = true
			err = spool.Commit(a.out)
			return committed, err
		}
		headingWritten := false
		err := pageItems(ctx, plan, boundFetch, func(items []pixiv.UserPreview) error {
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

// printUserPreviews 保留文本列表的一行一个用户格式，并将 stdout 失败交给调用方。
func printUserPreviews(w io.Writer, users []pixiv.UserPreview) error {
	for _, item := range users {
		if _, err := fmt.Fprintf(w, "%d %s\n", item.User.ID, item.User.Name); err != nil {
			return err
		}
	}
	return nil
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

// marshalJSONValue 序列化 CLI 输出值。SDK 模型的零值 sdk.Resource 会让默认
// MarshalJSON 报错；遇到这种错误时回退到记录协议序列化（把零资源视为 null），
// 从而保持含 json tag 的 DTO 输出不变，同时允许无 tag 的 SDK 模型安全输出。
func marshalJSONValue(value any, indent bool) ([]byte, error) {
	var (
		body []byte
		err  error
	)
	if indent {
		body, err = json.MarshalIndent(value, "    ", "  ")
	} else {
		body, err = json.Marshal(value)
	}
	if err == nil {
		return body, nil
	}
	return recordpkg.MarshalRecordValue(value)
}

func appendJSONArray[T any](s *jsonArraySpool, items []T) error {
	for _, item := range items {
		if s.first {
			s.first = false
		} else if _, err := io.WriteString(s.file, ","); err != nil {
			return err
		}
		encoded, err := marshalJSONValue(item, true)
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
