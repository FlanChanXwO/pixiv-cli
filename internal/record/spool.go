package record

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/pagination"
)

// Spool 暂存完整输出；成功后 Commit 才把内容交给 stdout。
type Spool struct{ *os.File }

// NewSpool 创建权限为 0600 的临时文档。
func NewSpool() (*Spool, error) {
	file, err := os.CreateTemp("", "pixiv-cli-json-*.tmp")
	if err != nil {
		return nil, err
	}
	return &Spool{file}, nil
}

// Commit 从头复制暂存内容到 out。
func (s *Spool) Commit(out io.Writer) error {
	if _, err := s.Seek(0, io.SeekStart); err != nil {
		return err
	}
	_, err := io.Copy(out, s.File)
	return err
}

// Close 关闭并删除临时文档。
func (s *Spool) Close() {
	if s == nil {
		return
	}
	name := s.Name()
	_ = s.File.Close()
	_ = os.Remove(name)
}

// JSONArraySpool 逐条暂存数组，避免把全部详情或列表保存在内存中。
type JSONArraySpool struct {
	*Spool
	first bool
}

// NewJSONArraySpool 创建一个尚未结束的 JSON 数组。
func NewJSONArraySpool() (*JSONArraySpool, error) {
	spool, err := NewSpool()
	if err != nil {
		return nil, err
	}
	if _, err := io.WriteString(spool, "["); err != nil {
		spool.Close()
		return nil, err
	}
	return &JSONArraySpool{Spool: spool, first: true}, nil
}

// Write 追加一个已编码的 JSON 值，并补上数组分隔符。
func (s *JSONArraySpool) Write(value []byte) (int, error) {
	if !s.first {
		if _, err := io.WriteString(s.Spool, ","); err != nil {
			return 0, err
		}
	}
	s.first = false
	return s.Spool.Write(value)
}

// AppendJSONArray 编码并追加一批数组元素。
func AppendJSONArray[T any](s *JSONArraySpool, items []T) error {
	for _, item := range items {
		body, err := json.Marshal(item)
		if err != nil {
			return err
		}
		if _, err := s.Write(body); err != nil {
			return err
		}
	}
	return nil
}

// Commit 结束数组后复制到 out。
func (s *JSONArraySpool) Commit(out io.Writer) error {
	if _, err := io.WriteString(s.Spool, "]\n"); err != nil {
		return err
	}
	return s.Spool.Commit(out)
}

// CommitList 结束数组，并写出与 NDJSON 共用元素的 records/pagination 文档。
func (s *JSONArraySpool) CommitList(out io.Writer, page pagination.PaginationOut) error {
	body, err := json.Marshal(page)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(s.Spool, "],\"pagination\":%s}\n", body); err != nil {
		return err
	}
	if _, err := s.Seek(0, io.SeekStart); err != nil {
		return err
	}
	_, err = io.Copy(out, io.MultiReader(strings.NewReader(`{"records":`), s.File))
	return err
}

// PageMetadata 根据分页计划保留显式 limit 与实际返回条数。
func PageMetadata(plan pagination.PagePlan, result pagination.PageResult) pagination.PaginationOut {
	var limit *int
	if !plan.OneBatch {
		limit = &plan.Limit
	}
	return pagination.ListPagination(plan, limit, result.Returned, result.HasMore)
}

// ListOutput 写出列表文档；空列表编码为 []。
type ListOutput[T any] struct {
	Records    []T                      `json:"records"`
	Pagination pagination.PaginationOut `json:"pagination"`
}

// NewListOutput 把分页结果和已投影的记录组成列表文档。
func NewListOutput[T any](items []T, plan pagination.PagePlan, result pagination.PageResult) ListOutput[T] {
	if items == nil {
		items = []T{}
	}
	return ListOutput[T]{Records: items, Pagination: PageMetadata(plan, result)}
}

// Close 清理尚未提交或已经提交的数组暂存。
func (s *JSONArraySpool) Close() {
	if s != nil {
		s.Spool.Close()
	}
}
