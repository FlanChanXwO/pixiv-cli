package vectorindex

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

const bookmarkBatchPages = 100

type scanPhase string

const (
	scanInitial   scanPhase = "initial"
	scanHead      scanPhase = "head"
	scanReconcile scanPhase = "reconcile"
)

type bookmarkState struct {
	Cursor   sdk.Cursor `json:"cursor"`
	Complete bool       `json:"complete"`
	reset    bool
}

type bookmarkScan struct {
	scope     pixiv.Restrict
	phase     scanPhase
	state     bookmarkState
	seen      map[string]bool
	restarted bool
}

func bookmarkKey(id int64, scope pixiv.Restrict, phase scanPhase) string {
	key := fmt.Sprintf("bookmarks/%d/%s", id, scope)
	if phase != scanInitial {
		key += "/" + string(phase)
	}
	return key
}

func (s *Store) readBookmarkState(ctx context.Context, id int64, scope pixiv.Restrict, phase scanPhase) (bookmarkState, error) {
	var body string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM state WHERE key=?`, bookmarkKey(id, scope, phase)).Scan(&body)
	if errors.Is(err, sql.ErrNoRows) {
		return bookmarkState{}, nil
	}
	if err != nil {
		return bookmarkState{}, err
	}
	var state bookmarkState
	if err = json.Unmarshal([]byte(body), &state); err != nil {
		if sdk.ReasonOf(err) == sdk.InvalidArgument {
			slog.WarnContext(ctx, "Restarting bookmarks after invalid saved cursor")
			return bookmarkState{reset: true}, nil
		}
		return bookmarkState{}, fmt.Errorf("invalid bookmark sync state: %w", err)
	}
	return state, nil
}

func saveBookmarkState(ctx context.Context, tx *sql.Tx, id int64, scan *bookmarkScan) error {
	body, err := json.Marshal(scan.state)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO state VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, bookmarkKey(id, scan.scope, scan.phase), string(body))
	return err
}

func (s *Store) prepareScans(ctx context.Context, id int64, options SyncOptions) ([]*bookmarkScan, error) {
	var scans []*bookmarkScan
	if options.headOnly {
		read, err := s.bookmarksRead(ctx, id)
		if err != nil || !read {
			return scans, err
		}
	}
	for _, scope := range []pixiv.Restrict{pixiv.RestrictPublic, pixiv.RestrictPrivate} {
		if options.Restrict != "" && options.Restrict != "all" && options.Restrict != string(scope) {
			continue
		}
		state, err := s.readBookmarkState(ctx, id, scope, scanInitial)
		if err != nil {
			return nil, err
		}
		phase := scanInitial
		if options.Reconcile {
			phase = scanReconcile
		} else if state.Complete {
			phase = scanHead
		}
		if phase != scanInitial {
			state, err = s.readBookmarkState(ctx, id, scope, phase)
			if err != nil {
				return nil, err
			}
		}
		scan := &bookmarkScan{scope: scope, phase: phase, state: state, seen: map[string]bool{}}
		if state.reset || phase == scanHead && state.Complete {
			if err = s.restartScan(ctx, id, scan); err != nil {
				return nil, err
			}
		}
		scans = append(scans, scan)
	}
	return scans, nil
}

func (s *Store) restartScan(ctx context.Context, id int64, scan *bookmarkScan) (err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if rollback := tx.Rollback(); !errors.Is(rollback, sql.ErrTxDone) {
			err = errors.Join(err, rollback)
		}
	}()
	if scan.phase == scanReconcile || scan.state.Complete {
		if _, err = tx.ExecContext(ctx, `DELETE FROM scan_seen WHERE account_id=? AND restrict=? AND phase=?`, id, string(scan.scope), string(scan.phase)); err != nil {
			return err
		}
	}
	next := *scan
	next.state = bookmarkState{}
	if err = saveBookmarkState(ctx, tx, id, &next); err != nil {
		return err
	}
	if err = tx.Commit(); err == nil {
		scan.state = next.state
	}
	return err
}

// shortcut: 头部整页均为旧收藏时停止；中段变动通过显式完整对账收敛。
func (s *Store) commitBookmarkPage(ctx context.Context, id int64, scan *bookmarkScan, page sdk.Page[*pixiv.Artwork], report *SyncReport) (err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if rollback := tx.Rollback(); !errors.Is(rollback, sql.ErrTxDone) {
			err = errors.Join(err, rollback)
		}
	}()
	allKnown, added, recorded, unavailable := true, 0, 0, 0
	for _, a := range page.Items {
		value, ok, err := project(a, time.Now().UTC())
		if err != nil {
			return err
		}
		if !ok {
			unavailable++
			allKnown = false
			continue
		}
		var known bool
		if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM bookmark WHERE account_id=? AND restrict=? AND artwork_id=?) AND NOT EXISTS(SELECT 1 FROM scan_seen WHERE account_id=? AND restrict=? AND phase=? AND artwork_id=? AND was_known=0)`, id, string(scan.scope), value.Record.ID(), id, string(scan.scope), string(scan.phase), value.Record.ID()).Scan(&known); err != nil {
			return err
		}
		allKnown = allKnown && known
		if err = putEntry(ctx, tx, value); err != nil {
			return err
		}
		if scan.phase != scanInitial {
			if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO scan_seen VALUES(?,?,?,?,?)`, id, string(scan.scope), string(scan.phase), value.Record.ID(), known); err != nil {
				return err
			}
		}
		result, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO bookmark VALUES(?,?,?)`, id, string(scan.scope), value.Record.ID())
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return err
		}
		added += int(n)
		recorded++
	}
	next := *scan
	next.state = bookmarkState{Cursor: page.Cursor, Complete: page.Cursor.IsZero() || scan.phase == scanHead && allKnown}
	if err = saveBookmarkState(ctx, tx, id, &next); err != nil {
		return err
	}
	if next.state.Complete && scan.phase == scanReconcile {
		initial := bookmarkScan{scope: scan.scope, phase: scanInitial, state: bookmarkState{Complete: true}}
		if err = saveBookmarkState(ctx, tx, id, &initial); err != nil {
			return err
		}
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	scan.state = next.state
	report.Bookmarks.Checked += len(page.Items)
	report.Bookmarks.New += added
	report.Bookmarks.Unavailable += unavailable
	report.Recorded += recorded
	report.Bookmarks.Pages++
	return nil
}

type bookmarkResult struct {
	scan *bookmarkScan
	page sdk.Page[*pixiv.Artwork]
	err  error
}

// scanBatch 的工作协程只读取上游；协调者串行提交页面后才派发该列表的下一页。
func (s *Store) scanBatch(ctx context.Context, client *pixiv.Client, id int64, scans []*bookmarkScan, deadline time.Time, report *SyncReport) error {
	ctx, cancel := context.WithCancel(ctx)
	var workers sync.WaitGroup
	defer func() { cancel(); workers.Wait() }()
	results := make(chan bookmarkResult, len(scans))
	jobs := make(map[*bookmarkScan]chan sdk.Cursor)
	for _, scan := range scans {
		queue := make(chan sdk.Cursor, 1)
		jobs[scan] = queue
		workers.Go(func() {
			for {
				select {
				case <-ctx.Done():
					return
				case cursor := <-queue:
					page, err := client.User(id).ArtworkBookmarks(ctx, pixiv.ArtworkBookmarksOptions{Restrict: scan.scope, Cursor: cursor})
					results <- bookmarkResult{scan, page, err}
				}
			}
		})
	}
	sent, active := 0, 0
	budgetReached := false
	dispatch := func(scan *bookmarkScan) error {
		if !deadline.IsZero() && !time.Now().Before(deadline) {
			budgetReached = true
			return nil
		}
		if scan.seen[scan.state.Cursor.String()] {
			return errors.New("bookmark cursor repeated")
		}
		scan.seen[scan.state.Cursor.String()] = true
		jobs[scan] <- scan.state.Cursor
		sent++
		active++
		return nil
	}
	for _, scan := range scans {
		if !scan.state.Complete {
			if err := dispatch(scan); err != nil {
				return err
			}
		}
	}
	for active > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case result := <-results:
			active--
			scan := result.scan
			if result.err != nil {
				if !scan.state.Cursor.IsZero() && !scan.restarted && sdk.ReasonOf(result.err) == sdk.InvalidArgument {
					if err := s.restartScan(ctx, id, scan); err != nil {
						return err
					}
					scan.restarted, scan.seen = true, map[string]bool{}
				} else {
					return result.err
				}
			} else if err := s.commitBookmarkPage(ctx, id, scan, result.page, report); err != nil {
				return err
			}
			if !scan.state.Complete && sent < bookmarkBatchPages {
				if err := dispatch(scan); err != nil {
					return err
				}
			}
		}
	}
	if budgetReached {
		return errTimeLimit
	}
	return nil
}

func (s *Store) finishReconcile(ctx context.Context, id int64, scans []*bookmarkScan, report *SyncReport) (err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if rollback := tx.Rollback(); !errors.Is(rollback, sql.ErrTxDone) {
			err = errors.Join(err, rollback)
		}
	}()
	removed := 0
	for _, scan := range scans {
		result, err := tx.ExecContext(ctx, `DELETE FROM bookmark WHERE account_id=? AND restrict=? AND artwork_id NOT IN (SELECT artwork_id FROM scan_seen WHERE account_id=? AND restrict=? AND phase=?)`, id, string(scan.scope), id, string(scan.scope), string(scanReconcile))
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return err
		}
		removed += int(n)
		if _, err = tx.ExecContext(ctx, `DELETE FROM state WHERE key=?`, bookmarkKey(id, scan.scope, scanReconcile)); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `DELETE FROM scan_seen WHERE account_id=? AND restrict=? AND phase=?`, id, string(scan.scope), string(scanReconcile)); err != nil {
			return err
		}
	}
	if err = tx.Commit(); err == nil {
		report.Bookmarks.Removed += removed
	}
	return err
}

func (s *Store) bookmarksRead(ctx context.Context, id int64) (bool, error) {
	for _, scope := range []pixiv.Restrict{pixiv.RestrictPublic, pixiv.RestrictPrivate} {
		state, err := s.readBookmarkState(ctx, id, scope, scanInitial)
		if err != nil {
			return false, err
		}
		if !state.Complete {
			return false, nil
		}
	}
	return true, nil
}
