package vectorindex

import (
	"context"
	"encoding/json"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

func (s *Store) commitRangePage(ctx context.Context, id int64, scan *bookmarkScan, page sdk.Page[*pixiv.Artwork], options SyncOptions, report *SyncReport) (err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { err = rollbackRange(tx, err) }()
	next, nextReport := *scan.selection, *report
	window := *options.Range
	for i, artwork := range page.Items {
		position := next.Progress.Scanned + i
		if window.Tail == 0 && (position < window.Offset || window.Limit > 0 && position-window.Offset >= window.Limit) {
			continue
		}
		value, valid, err := project(artwork, time.Now().UTC())
		if err != nil {
			return err
		}
		var body []byte
		var artworkID string
		if valid {
			body, err = json.Marshal(value)
			if err != nil {
				return err
			}
			artworkID = value.Record.ID()
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO sync_selection(task,restrict,position,artwork_id,snapshot) VALUES(?,?,?,?,?)`, next.Key, string(scan.scope), position, artworkID, body); err != nil {
			return err
		}
		next.Progress.Selected++
		if window.Tail == 0 {
			if !valid {
				nextReport.Bookmarks.Unavailable++
				continue
			}
			if err = applyRangeEntry(ctx, tx, id, scan.scope, &next, position, value, options, &nextReport); err != nil {
				return err
			}
		}
	}
	next.Progress.Scanned += len(page.Items)
	next.Cursor = page.Cursor
	done := page.Cursor.IsZero()
	if window.Tail > 0 {
		if _, err = tx.ExecContext(ctx, `DELETE FROM sync_selection WHERE task=? AND restrict=? AND position<?`, next.Key, string(scan.scope), next.Progress.Scanned-window.Tail); err != nil {
			return err
		}
		next.Progress.Selected = min(window.Tail, next.Progress.Scanned)
	} else {
		next.Promoted = true
		done = done || window.Limit > 0 && next.Progress.Scanned >= window.Offset && next.Progress.Scanned-window.Offset >= window.Limit
	}
	if done {
		next.Progress.Phase = RangeEmbedding
	}
	if err = saveRangeScan(ctx, tx, scan.scope, &next); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	*scan.selection, scan.state = next, bookmarkState{Cursor: next.Cursor, Complete: done}
	nextReport.Bookmarks.Checked += len(page.Items)
	nextReport.Bookmarks.Pages++
	*report = nextReport
	if options.ScanProgress != nil {
		return options.ScanProgress(string(scan.scope), next.Progress)
	}
	return nil
}
