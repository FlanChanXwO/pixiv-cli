package vectorindex

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/i18n"
	"github.com/FlanChanXwO/pixiv-cli/internal/pagination"
	"github.com/FlanChanXwO/pixiv-cli/internal/record"
)

const (
	catchUpBudget = 30 * time.Second
	searchBatch   = 30
)

// SearchError 保留原始错误供 CLI 使用，并向 MCP 提供固定阶段分类。
type SearchError struct {
	Code  string
	Cause error
}

// Error 返回完整的原始诊断。
func (e *SearchError) Error() string { return e.Cause.Error() }

// Unwrap 保留取消与具体错误类型。
func (e *SearchError) Unwrap() error { return e.Cause }

var errTimeLimit = i18n.Errorf("error.vector_catch_up_time_limit_reached")

// CatchUpStatus 标识搜索前的补算结果。
type CatchUpStatus string

const (
	CatchUpComplete  CatchUpStatus = "complete"
	CatchUpTimeLimit CatchUpStatus = "time_limit"
	CatchUpBusy      CatchUpStatus = "busy"
	CatchUpSkipped   CatchUpStatus = "skipped"
	CatchUpStopped   CatchUpStatus = "stopped"
)

// SearchInput 表示文字或图片查询；Page、Limit 采用共享列表分页契约。
type SearchInput struct {
	Query       string
	Image       []byte
	Page, Limit *int
	NoSync      bool
}

// SearchOptions 指定模型、固定账号和同步所需的网络能力。
type SearchOptions struct {
	SyncOptions
	AccountID int64
}

// SearchIndex 描述本次搜索覆盖范围和补算状态。
type SearchIndex struct {
	Model         string        `json:"model"`
	Pages         int           `json:"pages"`
	Pending       int           `json:"pending"`
	CatchUp       CatchUpStatus `json:"catch_up"`
	BookmarksRead bool          `json:"bookmarks_read"`
	Error         string        `json:"error,omitempty"`
}

// SearchResponse 的 Record 保留资源无关快照，附加最高分页面的 match 字段。
type SearchResponse struct {
	Records    []record.Record          `json:"records"`
	Pagination pagination.PaginationOut `json:"pagination"`
	Index      SearchIndex              `json:"index"`
}

// Search 在可用时补算，再扫描当前模型空间；补算失败仍可搜索已保存的向量。
func Search(ctx context.Context, root string, input SearchInput, options SearchOptions) (result SearchResponse, err error) {
	if (strings.TrimSpace(input.Query) == "") == (len(input.Image) == 0) {
		return result, i18n.Errorf("error.provide_exactly_one_text_query_or_image")
	}
	plan, err := pagination.ParseInput(input.Page, input.Limit)
	if err != nil {
		return result, err
	}
	code := "index_unavailable"
	defer func() {
		if err != nil {
			var classified *SearchError
			if !errors.As(err, &classified) {
				err = &SearchError{Code: code, Cause: err}
			}
		}
	}()
	var release func() error
	acquired := false
	result.Index.CatchUp = CatchUpSkipped
	if !input.NoSync {
		release, acquired, err = TrySyncLock(root)
		if err != nil {
			return result, err
		}
		if acquired {
			defer func() {
				if release != nil {
					err = errors.Join(err, release())
				}
			}()
		} else {
			result.Index.CatchUp = CatchUpBusy
		}
	}
	store, err := Open(ctx, root)
	if err != nil {
		return result, err
	}
	defer func() { err = errors.Join(err, store.Close()) }()
	var syncErr error
	if acquired {
		options.headOnly = true
		report, runErr := store.Sync(ctx, options.SyncOptions)
		syncErr = runErr
		result.Index.CatchUp = CatchUpComplete
		if report.timeLimit {
			result.Index.CatchUp = CatchUpTimeLimit
		}
		if syncErr != nil {
			result.Index.CatchUp, result.Index.Error = CatchUpStopped, ErrorCode(syncErr)
		}
		// 查询阶段释放操作通道与同步锁，让后续调用继续写入。
		err = release()
		release = nil
		if err != nil {
			return result, err
		}
	}
	if err = ctx.Err(); err != nil {
		return result, err
	}
	counts, err := store.Counts(ctx, options.Model)
	if err != nil {
		return result, err
	}
	result.Index.Model, result.Index.Pages, result.Index.Pending = options.Model, counts.Pages, counts.Pending
	result.Index.BookmarksRead, err = store.bookmarksRead(ctx, options.AccountID)
	if err != nil {
		return result, err
	}
	if counts.Pages == 0 && syncErr != nil {
		return result, &SearchError{Code: "sync_failed", Cause: syncErr}
	}
	if counts.Pages == 0 && !result.Index.BookmarksRead {
		return result, &SearchError{Code: "initial_sync_required", Cause: i18n.Errorf("error.vector_initial_sync_required")}
	}
	code = "model_unavailable"
	var vector []float32
	if len(input.Image) > 0 {
		vector, err = options.Embedder.Image(ctx, input.Image)
	} else {
		vector, err = options.Embedder.Text(ctx, input.Query)
	}
	if err != nil {
		return result, err
	}
	code = "index_unavailable"
	result.Records, result.Pagination, err = store.rank(ctx, options.Model, vector, plan, input.Limit)
	return result, err
}

type match struct {
	Score     float64 `json:"score"`
	PageIndex int     `json:"page_index"`
}

type candidate struct {
	id     int64
	record []byte
	match
}

// shortcut: 全表扫描适合数万页，达到几十万页时再引入近邻索引。
func (s *Store) rank(ctx context.Context, model string, query []float32, plan pagination.PagePlan, limit *int) (records []record.Record, page pagination.PaginationOut, err error) {
	rows, err := s.db.QueryContext(ctx, `SELECT a.id,a.record,e.page_index,e.vector FROM embedding e JOIN artwork a ON a.id=e.artwork_id WHERE e.model=? AND e.vector IS NOT NULL ORDER BY e.page_index`, model)
	if err != nil {
		return nil, page, err
	}
	defer func() { err = errors.Join(err, rows.Close()) }()
	best := map[int64]candidate{}
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return nil, page, err
		}
		var c candidate
		var id string
		var vector []byte
		if err = rows.Scan(&id, &c.record, &c.PageIndex, &vector); err != nil {
			return nil, page, err
		}
		if c.id, err = strconv.ParseInt(id, 10, 64); err != nil {
			return nil, page, err
		}
		if len(query) == 0 || len(vector) != len(query)*4 {
			return nil, page, i18n.Errorf("error.model_vector_dimensions_changed")
		}
		for i, value := range query {
			v := math.Float32frombits(binary.LittleEndian.Uint32(vector[i*4:]))
			if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
				return nil, page, i18n.Errorf("error.invalid_stored_vector")
			}
			c.Score += float64(value) * float64(v)
		}
		if previous, ok := best[c.id]; !ok || c.Score > previous.Score {
			best[c.id] = c
		}
	}
	if err = rows.Err(); err != nil {
		return nil, page, err
	}
	sorted := make([]candidate, 0, len(best))
	for _, c := range best {
		sorted = append(sorted, c)
	}
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Score != sorted[j].Score {
			return sorted[i].Score > sorted[j].Score
		}
		return sorted[i].id > sorted[j].id
	})
	start := min(plan.Skip, len(sorted))
	end := len(sorted)
	if plan.OneBatch {
		end = min(end, start+searchBatch)
	} else if plan.Limit > 0 {
		end = start + min(plan.Limit, end-start)
	}
	records = make([]record.Record, 0, end-start)
	for _, c := range sorted[start:end] {
		var fields map[string]json.RawMessage
		if err = json.Unmarshal(c.record, &fields); err != nil {
			return nil, page, err
		}
		if fields["match"], err = json.Marshal(c.match); err != nil {
			return nil, page, err
		}
		body, err := json.Marshal(fields)
		if err != nil {
			return nil, page, err
		}
		r, err := record.ParseRecordJSON(body)
		if err != nil {
			return nil, page, err
		}
		records = append(records, r)
	}
	return records, pagination.ListPagination(plan, limit, len(records), end < len(sorted)), nil
}
