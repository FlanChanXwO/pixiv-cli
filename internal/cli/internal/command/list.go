// 列表命令共用的逻辑分页参数与 JSON 文档暂存；Pixiv 与 FANBOX 列表都经由这里。

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

// Plan is the resolved logical page selection.
type Plan struct {
	limit    int
	skip     int
	oneBatch bool
}

// AllResults returns the plan used by workflows that must visit every page.
func AllResults() Plan { return Plan{} }

// PagePlan exposes the resolved selection to commands that drive shared
// pagination directly.
func (p Plan) PagePlan() pagination.PagePlan {
	return pagination.PagePlan{Skip: p.skip, Limit: p.limit, OneBatch: p.oneBatch}
}

// PushDownSkip hands a logical skip to an upstream offset only when raw and
// logical results correspond one-to-one.
func (p Plan) PushDownSkip() (Plan, int) {
	offset := p.skip
	p.skip = 0
	return p, offset
}

// ParsePlan validates a command's logical pagination settings.
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

// PageItems hands a CLI page plan to shared pagination for commands that run
// their own inline read.
func PageItems[T any](ctx context.Context, plan Plan, fetch func(context.Context, sdk.Cursor) ([]T, sdk.Cursor, error), consume func([]T) error) error {
	_, err := pagination.TraversePages(ctx, plan.PagePlan(), fetch, consume)
	return err
}

// JSONArraySpool writes each confirmed element of a single-key list document
// to a private temporary file. Network or encoding failures leave stdout
// untouched; Commit then transfers one complete JSON document.
type JSONArraySpool struct {
	file  *os.File
	first bool
}

// NewJSONArraySpool starts the document `{"key": [`.
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

// AppendJSONArray appends items as indented array elements.
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

// Commit closes the document and copies it to out.
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

// Close removes the temporary file.
func (s *JSONArraySpool) Close() {
	name := s.file.Name()
	_ = s.file.Close()
	_ = os.Remove(name)
}
