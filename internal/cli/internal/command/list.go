package command

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"

	"github.com/FlanChanXwO/pixiv-cli/internal/pagination"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/spf13/cobra"
)

// Plan 保存解析后的逻辑分页选择：每批条数、跳过的条数，以及是否只读一批。
type Plan struct {
	limit    int
	skip     int
	oneBatch bool
}

// AllResults 返回遍历全部分页的计划，不限制条数也不跳过。
func AllResults() Plan { return Plan{} }

// PagePlan 把逻辑选择转换成共享分页参数，供自行驱动分页的命令使用。
func (p Plan) PagePlan() pagination.PagePlan {
	return pagination.PagePlan{Skip: p.skip, Limit: p.limit, OneBatch: p.oneBatch}
}

// PushDownSkip 把逻辑跳过量转成上游 offset，并清空本地 skip。只有上游原始结果与
// 逻辑结果一一对应时才能这样调用。
func (p Plan) PushDownSkip() (Plan, int) {
	offset := p.skip
	p.skip = 0
	return p, offset
}

// ParsePlan 校验 --limit/--page 的组合并生成分页计划；未显式给 --limit 时只读一批。
func ParsePlan(cmd *cobra.Command, limit, page int) (Plan, error) {
	if limit < 0 {
		return Plan{}, errors.New("limit must be zero or a positive integer")
	}
	if cmd.Flags().Changed("page") && page <= 0 {
		return Plan{}, errors.New("page must be a positive integer")
	}
	if page > 0 {
		if limit <= 0 {
			return Plan{}, errors.New("--page requires --limit to be a positive integer")
		}
		if page-1 > math.MaxInt/limit {
			return Plan{}, errors.New("page and limit overflow the logical result offset")
		}
		return Plan{limit: limit, skip: (page - 1) * limit}, nil
	}
	return Plan{limit: limit, oneBatch: !cmd.Flags().Changed("limit")}, nil
}

// PageItems 把分页计划交给共享遍历，供自带内联读取的命令使用。
func PageItems[T any](ctx context.Context, plan Plan, fetch func(context.Context, sdk.Cursor) ([]T, sdk.Cursor, error), consume func([]T) error) error {
	_, err := pagination.TraversePages(ctx, plan.PagePlan(), fetch, consume)
	return err
}

// JSONArraySpool 把单键列表文档的每个已确认元素写进私有临时文件；网络或编码失败时
// stdout 保持为空，Commit 之后一次性写出完整 JSON。
type JSONArraySpool struct {
	file  *os.File
	first bool
}

// NewJSONArraySpool 创建临时文件并写入文档开头 `{"key": [`。
func NewJSONArraySpool(key string) (*JSONArraySpool, error) {
	file, err := os.CreateTemp("", "pixiv-cli-json-*.tmp")
	if err != nil {
		return nil, err
	}
	if _, err := fmt.Fprintf(file, "{\n  %q: [", key); err != nil {
		_ = file.Close()
		_ = os.Remove(file.Name())
		return nil, err
	}
	return &JSONArraySpool{file: file, first: true}, nil
}

// AppendJSONArray 把一批元素按两空格缩进追加进数组。
func AppendJSONArray[T any](s *JSONArraySpool, items []T) error {
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

// Commit 写完文档结尾，并把完整内容复制到 out。
func (s *JSONArraySpool) Commit(out io.Writer) error {
	if _, err := io.WriteString(s.file, "\n  ]\n}\n"); err != nil {
		return err
	}
	if _, err := s.file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	_, err := io.Copy(out, s.file)
	return err
}

// Close 关闭并删除临时文件；无论 Commit 是否执行都调用。
func (s *JSONArraySpool) Close() {
	name := s.file.Name()
	_ = s.file.Close()
	_ = os.Remove(name)
}
