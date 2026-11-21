package vectorindex

import (
	"context"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"io"
	"math"
	"net/http"
	"sort"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/searchfilter"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

// Embedder 提供同一模型空间内的归一化图片和文字向量。
type Embedder interface {
	Image(context.Context, []byte) ([]float32, error)
	Text(context.Context, string) ([]float32, error)
	Close() error
}

// ClientOpener 打开固定账号的 client 并取得操作通道；close 释放通道和连接。
type ClientOpener func(context.Context) (accountID int64, client *pixiv.Client, close func() error, err error)

// SyncOptions 指定当前向量身份、模型和不带记录回调的账号 client。
type SyncOptions struct {
	Model        string
	Embedder     Embedder
	OpenClient   ClientOpener
	ProbeImage   []byte
	Progress     func(done, total int) error
	ScanProgress func(scope string, progress RangeProgress) error // 每页持久化后由协调者调用。
	// Restrict 选择收藏范围；有效筛选同时限制计算，零值计算共享索引全部待算页。
	Restrict     string
	Filter       searchfilter.Filter
	Reconcile    bool
	Range        *Range // nil 沿用普通同步；非 nil 仅计算选区内的待算页。
	accountID    int64
	artworkID    string
	headOnly     bool
	deadline     time.Time
	selectionKey string
}

// SyncReport 保留已持久化的进度；Stopped 非空时调用方仍须返回原错误。
type SyncReport struct {
	Range     *RangeReport `json:"range,omitempty"`
	Model     string       `json:"model"`
	Bookmarks struct {
		Checked     int                         `json:"checked"`
		New         int                         `json:"new"`
		Pages       int                         `json:"pages"`
		Unavailable int                         `json:"unavailable"`
		Removed     int                         `json:"removed"`
		Scans       map[string]BookmarkProgress `json:"scans"`
	} `json:"bookmarks"`
	MergeReport
	Recorded    int     `json:"recorded"`
	Embedded    int     `json:"embedded"`
	Unavailable int     `json:"unavailable"`
	Failed      int     `json:"failed"`
	Stopped     *string `json:"stopped"`
	Index       Counts  `json:"index"`
	timeLimit   bool
}

// BookmarkProgress 描述所选收藏范围的扫描阶段与完成状态。
type BookmarkProgress struct {
	Phase    string `json:"phase"`
	Complete bool   `json:"complete"`
}

// Sync 合并记录，并在每批收藏页提交后计算向量；调用方须持有同步锁。
func (s *Store) Sync(ctx context.Context, options SyncOptions) (report SyncReport, err error) {
	report.Model = options.Model
	var scans []*bookmarkScan
	defer func() {
		report.Bookmarks.Scans = make(map[string]BookmarkProgress)
		for _, scan := range scans {
			if scan.selection != nil {
				report.Range.Scans[string(scan.scope)] = scan.selection.Progress
			} else {
				report.Bookmarks.Scans[string(scan.scope)] = BookmarkProgress{string(scan.phase), scan.state.Complete}
			}
		}
		var countErr error
		report.Index, countErr = s.Counts(context.WithoutCancel(ctx), options.Model)
		err = errors.Join(err, countErr)
		if err != nil {
			code := ErrorCode(err)
			report.Stopped = &code
		}
	}()
	report.MergeReport, err = s.Merge(ctx)
	if err != nil {
		return report, err
	}
	if options.headOnly {
		options.deadline = time.Now().Add(catchUpBudget)
	}
	accountID, client, closeClient, err := options.OpenClient(ctx)
	if err != nil {
		return report, err
	}
	defer func() { err = errors.Join(err, closeClient()) }()
	defer func() {
		if errors.Is(err, errTimeLimit) {
			report.timeLimit, err = true, nil
		}
	}()
	options.accountID = accountID
	if options.Range != nil {
		report.Range = &RangeReport{Range: *options.Range, Scans: map[string]RangeProgress{}}
		scans, err = s.prepareRangeScans(ctx, accountID, options)
		if err == nil {
			options.selectionKey = scans[0].selection.Key
		}
	} else {
		scans, err = s.prepareScans(ctx, accountID, options)
	}
	if err != nil {
		return report, err
	}
	for {
		if err = s.scanBatch(ctx, client, accountID, scans, options, &report); err != nil {
			return report, err
		}
		complete := true
		for _, scan := range scans {
			complete = complete && scan.state.Complete
		}
		if complete && options.Reconcile {
			if err = s.finishReconcile(ctx, accountID, scans, &report); err != nil {
				return report, err
			}
		}
		if options.Range != nil && options.Range.Tail > 0 {
			for _, scan := range scans {
				if err = s.promoteTail(ctx, accountID, scan, options, &report); err != nil {
					return report, err
				}
			}
		}
		if err = s.embedPages(ctx, client, options, &report); err != nil {
			return report, err
		}
		if options.Range != nil {
			if err = s.finishRangeBatch(ctx, scans, complete); err != nil {
				return report, err
			}
		}
		if complete {
			return report, nil
		}
	}
}

// embeddingError 保留模型错误链，区分云端模型与 Pixiv 的限流。
type embeddingError struct{ error }

func (e *embeddingError) Unwrap() error { return e.error }

// ErrorCode 给同步摘要提供稳定原因，不包含图片地址或上游正文。
func ErrorCode(err error) string {
	var modelError *embeddingError
	if errors.As(err, &modelError) && httpStatus(modelError.error) == http.StatusTooManyRequests {
		return "embedding_rate_limited"
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return "canceled"
	}
	status := httpStatus(err)
	switch status {
	case 401:
		return "unauthorized"
	case 403:
		return "forbidden"
	case 429:
		return "rate_limited"
	}
	var upstream *sdk.Error
	if errors.As(err, &upstream) {
		return string(upstream.Reason)
	}
	if status != 0 {
		return "upstream_error"
	}
	return "sync_failed"
}

func httpStatus(err error) int {
	var status interface{ StatusCode() int }
	if errors.As(err, &status) {
		return status.StatusCode()
	}
	var upstream *sdk.Error
	if errors.As(err, &upstream) {
		return upstream.HTTPStatus
	}
	return 0
}

type pendingPage struct {
	id    string
	index int
	url   string
	at    time.Time
}

func (s *Store) pendingPages(ctx context.Context, options SyncOptions) (pages []pendingPage, err error) {
	rows, err := s.db.QueryContext(ctx, `SELECT p.artwork_id,p.page_index,p.url,a.recorded_at,a.record FROM page p JOIN artwork a ON a.id=p.artwork_id LEFT JOIN embedding e ON e.artwork_id=p.artwork_id AND e.page_index=p.page_index AND e.model=? WHERE p.unavailable='' AND e.model IS NULL AND (?='' OR p.artwork_id=?) AND (?=0 OR EXISTS(SELECT 1 FROM bookmark b WHERE b.account_id=? AND b.artwork_id=a.id AND b.restrict IN ('public','private') AND (? IN ('','all') OR b.restrict=?))) AND (?='' OR EXISTS(SELECT 1 FROM sync_selection c WHERE c.task=? AND c.artwork_id=p.artwork_id AND c.matched=1))`, options.Model, options.artworkID, options.artworkID, options.scoped(), options.accountID, options.Restrict, options.Restrict, options.selectionKey, options.selectionKey)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, rows.Close()) }()
	for rows.Next() {
		var p pendingPage
		var at string
		var body []byte
		if err = rows.Scan(&p.id, &p.index, &p.url, &at, &body); err != nil {
			return nil, err
		}
		if p.at, err = time.Parse(time.RFC3339Nano, at); err != nil {
			return nil, fmt.Errorf("invalid vector record timestamp: %w", err)
		}
		var snapshot struct {
			ContentType string `json:"content_type"`
			XRestrict   int    `json:"x_restrict"`
		}
		if err = json.Unmarshal(body, &snapshot); err != nil {
			return nil, err
		}
		if options.Filter.Matches(snapshot.XRestrict, snapshot.ContentType) {
			pages = append(pages, p)
		}
	}
	sort.SliceStable(pages, func(i, j int) bool { return pages[i].at.After(pages[j].at) })
	return pages, rows.Err()
}

func (o SyncOptions) scoped() bool {
	return o.Range != nil || o.Restrict != "" && o.Restrict != "all" || o.Filter.Rating != "" && o.Filter.Rating != searchfilter.RatingAll || o.Filter.ContentType != "" && o.Filter.ContentType != searchfilter.ContentTypeAll
}

func (s *Store) embedPages(ctx context.Context, client *pixiv.Client, options SyncOptions, report *SyncReport) error {
	pages, err := s.pendingPages(ctx, options)
	if err != nil {
		return err
	}
	probed := false
	total := report.Embedded + report.Unavailable + report.Failed + len(pages)
	for _, p := range pages {
		if !options.deadline.IsZero() && !time.Now().Before(options.deadline) {
			return errTimeLimit
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		data, status, err := downloadPage(ctx, client, p.url)
		if err != nil {
			return err
		}
		if status == http.StatusNotFound {
			if _, err = s.db.ExecContext(ctx, `UPDATE page SET unavailable='not_found' WHERE artwork_id=? AND page_index=?`, p.id, p.index); err != nil {
				return err
			}
			report.Unavailable++
		} else {
			vector, embedErr := options.Embedder.Image(ctx, data)
			if httpStatus(embedErr) == http.StatusBadRequest {
				if !probed {
					if _, err = options.Embedder.Image(ctx, options.ProbeImage); err != nil {
						return &embeddingError{err}
					}
					probed = true
				}
			} else if embedErr != nil && !errors.Is(embedErr, image.ErrFormat) {
				return &embeddingError{embedErr}
			}
			if embedErr != nil {
				if _, err = s.db.ExecContext(ctx, `INSERT INTO embedding(artwork_id,page_index,model,failure) VALUES(?,?,?,'unsupported_image')`, p.id, p.index, options.Model); err != nil {
					return err
				}
				report.Failed++
			} else {
				if err = s.saveVector(ctx, p, options.Model, vector); err != nil {
					return err
				}
				report.Embedded++
			}
		}
		if options.Progress != nil {
			if err = options.Progress(report.Embedded+report.Unavailable+report.Failed, total); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Store) saveVector(ctx context.Context, p pendingPage, model string, vector []float32) error {
	var size int
	if err := s.db.QueryRowContext(ctx, `SELECT length(vector) FROM embedding WHERE model=? AND vector IS NOT NULL LIMIT 1`, model).Scan(&size); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if size != 0 && size != len(vector)*4 {
		return errors.New("model vector dimensions changed")
	}
	body := make([]byte, len(vector)*4)
	for i, v := range vector {
		binary.LittleEndian.PutUint32(body[i*4:], math.Float32bits(v))
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO embedding(artwork_id,page_index,model,vector) VALUES(?,?,?,?)`, p.id, p.index, model, body)
	return err
}

func downloadPage(ctx context.Context, client *pixiv.Client, raw string) ([]byte, int, error) {
	original, err := client.Image(raw)
	if err != nil {
		return nil, 0, err
	}
	if regular, deriveErr := original.Quality(pixiv.ImageQualityRegular); deriveErr == nil {
		data, status, err := readImage(ctx, regular)
		if err != nil || status != http.StatusNotFound {
			return data, status, err
		}
	}
	return readImage(ctx, original)
}

func readImage(ctx context.Context, img pixiv.Image) (data []byte, status int, err error) {
	response, err := img.OpenResponse(ctx)
	if err != nil {
		return nil, 0, err
	}
	defer func() { err = errors.Join(err, response.Body.Close()) }()
	if response.StatusCode == http.StatusNotFound {
		return nil, response.StatusCode, nil
	}
	if response.StatusCode != http.StatusOK {
		return nil, response.StatusCode, &sdk.Error{Reason: sdk.UpstreamError, Operation: "vector image", HTTPStatus: response.StatusCode, Message: fmt.Sprintf("HTTP %d", response.StatusCode)}
	}
	data, err = io.ReadAll(response.Body)
	return data, response.StatusCode, err
}
