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
		UserID:             client.UserID,
		RefreshToken:       client.RefreshToken,
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

func (a app) runIllustList(ctx context.Context, plan listPlan, jsonOut bool, fetch func(context.Context, sdk.Cursor) ([]sdk.Illust, sdk.Cursor, error), print func([]sdk.Illust, int)) error {
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
		print(items, position)
		position += len(items)
		return nil
	})
}

func (a app) runUserList(ctx context.Context, plan listPlan, jsonOut bool, fetch func(context.Context, sdk.Cursor) ([]sdk.UserPreview, sdk.Cursor, error), print func([]sdk.UserPreview)) error {
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
	return pageItems(ctx, plan, fetch, func(items []sdk.UserPreview) error { print(items); return nil })
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
