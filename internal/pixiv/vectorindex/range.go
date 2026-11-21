package vectorindex

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/FlanChanXwO/pixiv-cli/internal/i18n"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/searchfilter"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

const rangePrefix = "range/"

// Range 先按每份收藏列表的原始位置选区，再应用内容筛选。零值选取整份列表。
// Offset 从零开始，Limit 为零时读到末尾；正数 Tail 与 Offset、Limit 互斥。
type Range struct {
	Offset int `json:"offset"`
	Limit  int `json:"limit"`
	Tail   int `json:"tail"`
}

// Validate 检查外部范围参数；显式 Tail 为零及参数拼写冲突由命令入口处理。
func (r Range) Validate() error {
	if r.Offset < 0 || r.Limit < 0 || r.Tail < 0 || r.Tail > 0 && (r.Offset != 0 || r.Limit != 0) {
		return i18n.Errorf("error.vector_range")
	}
	return nil
}

// RangePhase 表示范围任务正在定位、计算或已完成。
type RangePhase string

// RangeScanning 等值描述范围任务的持久化阶段。
const (
	RangeScanning  RangePhase = "scanning"
	RangeEmbedding RangePhase = "embedding"
	RangeComplete  RangePhase = "complete"
)

// RangeProgress 按作品位置统计扫描、选取和匹配数，ReusedPages 按图片页计数。
// 计数跨中断保留；尾部的 Matched、ReusedPages 在列表结束后确定。
type RangeProgress struct {
	Phase       RangePhase `json:"phase"`
	Scanned     int        `json:"scanned"`
	Selected    int        `json:"selected"`
	Matched     int        `json:"matched"`
	ReusedPages int        `json:"reused_pages"`
}

// RangeReport 展示本次选区及各可见性的累计进度，与全量收藏扫描状态分开。
type RangeReport struct {
	Range
	Scans map[string]RangeProgress `json:"scans"`
}

// rangeScan 的游标、计数和候选由协调者提交；扫描结束后才确定尾部集合。
type rangeScan struct {
	Key      string        `json:"-"`
	Cursor   sdk.Cursor    `json:"cursor"`
	Progress RangeProgress `json:"progress"`
	Promoted bool          `json:"promoted"`
}

func (s *Store) prepareRangeScans(ctx context.Context, id int64, options SyncOptions) ([]*bookmarkScan, error) {
	filter, err := searchfilter.NormalizeFilter(string(options.Filter.Rating), string(options.Filter.ContentType))
	if err != nil {
		return nil, err
	}
	restrict := options.Restrict
	if restrict == "" {
		restrict = "all"
	}
	identity := struct {
		AccountID int64
		Restrict  string
		Range     Range
		Filter    searchfilter.Filter
		Model     string
	}{id, restrict, *options.Range, filter, options.Model}
	body, err := json.Marshal(identity)
	if err != nil {
		return nil, err
	}
	key := rangePrefix + string(body)
	var scans []*bookmarkScan
	for _, scope := range []pixiv.Restrict{pixiv.RestrictPublic, pixiv.RestrictPrivate} {
		if restrict != "all" && restrict != string(scope) {
			continue
		}
		selection := &rangeScan{Key: key, Progress: RangeProgress{Phase: RangeScanning}}
		reset := false
		var saved string
		err := s.db.QueryRowContext(ctx, `SELECT value FROM state WHERE key=?`, key+"/"+string(scope)).Scan(&saved)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		if err == nil {
			if err := json.Unmarshal([]byte(saved), selection); err != nil {
				// 仅 SDK 明确标记的失效游标允许重扫；其余持久化错误原样报告。
				if sdk.ReasonOf(err) != sdk.InvalidArgument {
					return nil, err
				}
				reset = true
			}
		}
		scan := &bookmarkScan{scope: scope, selection: selection, state: bookmarkState{Cursor: selection.Cursor, Complete: selection.Progress.Phase != RangeScanning}, seen: map[string]bool{}}
		if reset {
			if err := s.restartRange(ctx, scan); err != nil {
				return nil, err
			}
		}
		scans = append(scans, scan)
	}
	return scans, nil
}

func saveRangeScan(ctx context.Context, tx *sql.Tx, scope pixiv.Restrict, selection *rangeScan) error {
	body, err := json.Marshal(selection)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO state VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, selection.Key+"/"+string(scope), string(body))
	return err
}

func (s *Store) restartRange(ctx context.Context, scan *bookmarkScan) (err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { err = rollbackRange(tx, err) }()
	next := rangeScan{Key: scan.selection.Key, Progress: RangeProgress{Phase: RangeScanning}}
	if _, err = tx.ExecContext(ctx, `DELETE FROM sync_selection WHERE task=? AND restrict=?`, next.Key, string(scan.scope)); err != nil {
		return err
	}
	if err = saveRangeScan(ctx, tx, scan.scope, &next); err != nil {
		return err
	}
	if err = tx.Commit(); err == nil {
		*scan.selection, scan.state = next, bookmarkState{}
	}
	return err
}

func rollbackRange(tx *sql.Tx, err error) error {
	if rollback := tx.Rollback(); !errors.Is(rollback, sql.ErrTxDone) {
		return errors.Join(err, rollback)
	}
	return err
}

func (s *Store) promoteTail(ctx context.Context, id int64, scan *bookmarkScan, options SyncOptions, report *SyncReport) (err error) {
	if !scan.state.Complete || scan.selection.Promoted || scan.selection.Progress.Phase == RangeComplete {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { err = rollbackRange(tx, err) }()
	rows, err := tx.QueryContext(ctx, `SELECT position,snapshot FROM sync_selection WHERE task=? AND restrict=? ORDER BY position`, scan.selection.Key, string(scan.scope))
	if err != nil {
		return err
	}
	next, nextReport := *scan.selection, *report
	for rows.Next() {
		var position int
		var body []byte
		if err = rows.Scan(&position, &body); err != nil {
			return errors.Join(err, rows.Close())
		}
		if len(body) == 0 {
			nextReport.Bookmarks.Unavailable++
			continue
		}
		var value entry
		if err = json.Unmarshal(body, &value); err != nil {
			return errors.Join(err, rows.Close())
		}
		if err = applyRangeEntry(ctx, tx, id, scan.scope, &next, position, value, options, &nextReport); err != nil {
			return errors.Join(err, rows.Close())
		}
	}
	if err = errors.Join(rows.Err(), rows.Close()); err != nil {
		return err
	}
	next.Promoted = true
	if err = saveRangeScan(ctx, tx, scan.scope, &next); err != nil {
		return err
	}
	if err = tx.Commit(); err == nil {
		*scan.selection = next
		*report = nextReport
	}
	return err
}

func applyRangeEntry(ctx context.Context, tx *sql.Tx, id int64, scope pixiv.Restrict, selection *rangeScan, position int, value entry, options SyncOptions, report *SyncReport) error {
	if err := putEntry(ctx, tx, value); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO bookmark VALUES(?,?,?)`, id, string(scope), value.Record.ID())
	if err != nil {
		return err
	}
	added, err := result.RowsAffected()
	if err != nil {
		return err
	}
	var snapshot struct {
		ContentType string `json:"content_type"`
		XRestrict   int    `json:"x_restrict"`
	}
	body, err := json.Marshal(value.Record)
	if err != nil {
		return err
	}
	if err = json.Unmarshal(body, &snapshot); err != nil {
		return err
	}
	if options.Filter.Matches(snapshot.XRestrict, snapshot.ContentType) {
		var reused int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM embedding WHERE artwork_id=? AND model=? AND vector IS NOT NULL`, value.Record.ID(), options.Model).Scan(&reused); err != nil {
			return err
		}
		selection.Progress.Matched++
		selection.Progress.ReusedPages += reused
		if _, err := tx.ExecContext(ctx, `UPDATE sync_selection SET matched=1 WHERE task=? AND restrict=? AND position=?`, selection.Key, string(scope), position); err != nil {
			return err
		}
	}
	report.Bookmarks.New += int(added)
	report.Recorded++
	return nil
}

func (s *Store) finishRangeBatch(ctx context.Context, scans []*bookmarkScan, complete bool) (err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { err = rollbackRange(tx, err) }()
	for _, scan := range scans {
		next := *scan.selection
		if scan.state.Complete {
			next.Progress.Phase = RangeComplete
		}
		if complete {
			if _, err = tx.ExecContext(ctx, `DELETE FROM state WHERE key=?`, next.Key+"/"+string(scan.scope)); err != nil {
				return err
			}
		} else if err = saveRangeScan(ctx, tx, scan.scope, &next); err != nil {
			return err
		}
	}
	if complete {
		if _, err = tx.ExecContext(ctx, `DELETE FROM sync_selection WHERE task=?`, scans[0].selection.Key); err != nil {
			return err
		}
	}
	if err = tx.Commit(); err == nil {
		for _, scan := range scans {
			if scan.state.Complete {
				scan.selection.Progress.Phase = RangeComplete
			}
		}
	}
	return err
}
