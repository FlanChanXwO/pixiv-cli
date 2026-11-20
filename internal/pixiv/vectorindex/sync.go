package vectorindex

import (
	"context"
	"database/sql"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"io"
	"math"
	"net/http"
	"sort"
	"time"

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
	Model      string
	Embedder   Embedder
	OpenClient ClientOpener
	ProbeImage []byte
	Progress   func(done, total int) error
	headOnly   bool
	deadline   time.Time
}

// SyncReport 保留已持久化的进度；Stopped 非空时调用方仍须返回原错误。
type SyncReport struct {
	Model     string `json:"model"`
	Bookmarks struct {
		Checked int `json:"checked"`
		New     int `json:"new"`
	} `json:"bookmarks"`
	MergeReport
	Embedded    int     `json:"embedded"`
	Unavailable int     `json:"unavailable"`
	Failed      int     `json:"failed"`
	Stopped     *string `json:"stopped"`
	Index       Counts  `json:"index"`
	timeLimit   bool
}

// Sync 合并记录、续读收藏并逐页提交结果；调用方须持有同步锁。
func (s *Store) Sync(ctx context.Context, options SyncOptions) (report SyncReport, err error) {
	report.Model = options.Model
	defer func() {
		var countErr error
		report.Index, countErr = s.Counts(context.WithoutCancel(ctx), options.Model)
		err = errors.Join(err, countErr)
		if err != nil {
			code := ErrorCode(err)
			report.Stopped = &code
		}
	}()
	// 1. 先恢复本地记录，账号不可用时仍保留已合并的数据。
	report.MergeReport, err = s.Merge(ctx)
	if err != nil {
		return report, err
	}
	if options.headOnly {
		options.deadline = time.Now().Add(catchUpBudget)
	}
	// 2. 固定账号并持有操作通道，公开与私密分别续读。
	accountID, client, closeClient, err := options.OpenClient(ctx)
	if err != nil {
		return report, err
	}
	defer func() { err = errors.Join(err, closeClient()) }()
	read, err := s.bookmarksRead(ctx, accountID)
	if err != nil {
		return report, err
	}
	for _, scope := range []pixiv.Restrict{pixiv.RestrictPublic, pixiv.RestrictPrivate} {
		if options.headOnly && !read {
			break
		}
		if err = s.syncBookmarks(ctx, client, accountID, scope, options.deadline, &report); err != nil {
			if errors.Is(err, errTimeLimit) {
				report.timeLimit = true
				return report, nil
			}
			return report, err
		}
	}
	// 3. 逐页提交；一次失败保留此前的处理结果。
	err = s.embedPages(ctx, client, options, &report)
	if errors.Is(err, errTimeLimit) {
		report.timeLimit, err = true, nil
	}
	return report, err
}

// ErrorCode 给同步摘要提供稳定原因，不包含图片地址或上游正文。
func ErrorCode(err error) string {
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

func (s *Store) pendingPages(ctx context.Context, model string) (pages []pendingPage, err error) {
	rows, err := s.db.QueryContext(ctx, `SELECT p.artwork_id,p.page_index,p.url,a.recorded_at FROM page p JOIN artwork a ON a.id=p.artwork_id LEFT JOIN embedding e ON e.artwork_id=p.artwork_id AND e.page_index=p.page_index AND e.model=? WHERE p.unavailable='' AND e.model IS NULL`, model)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, rows.Close()) }()
	for rows.Next() {
		var p pendingPage
		var at string
		if err = rows.Scan(&p.id, &p.index, &p.url, &at); err != nil {
			return nil, err
		}
		if p.at, err = time.Parse(time.RFC3339Nano, at); err != nil {
			return nil, fmt.Errorf("invalid vector record timestamp: %w", err)
		}
		pages = append(pages, p)
	}
	sort.SliceStable(pages, func(i, j int) bool { return pages[i].at.After(pages[j].at) })
	return pages, rows.Err()
}

func (s *Store) embedPages(ctx context.Context, client *pixiv.Client, options SyncOptions, report *SyncReport) error {
	pages, err := s.pendingPages(ctx, options.Model)
	if err != nil {
		return err
	}
	probed := false
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
						return err
					}
					probed = true
				}
			} else if embedErr != nil && !errors.Is(embedErr, image.ErrFormat) {
				return embedErr
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
			if err = options.Progress(report.Embedded+report.Unavailable+report.Failed, len(pages)); err != nil {
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
