package vectorindex

import (
	"context"
	"errors"
	"iter"
	"strconv"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/i18n"
)

// Add 顺序刷新作品详情并立即计算；输入和上游失败均保留此前已提交结果。
// 调用方须持有同步锁；重复 ID 在同次调用中只处理一次。
func (s *Store) Add(ctx context.Context, ids iter.Seq2[int64, error], options SyncOptions) (report SyncReport, err error) {
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
	report.MergeReport, err = s.Merge(ctx)
	if err != nil {
		return report, err
	}
	_, client, closeClient, err := options.OpenClient(ctx)
	if err != nil {
		return report, err
	}
	defer func() { err = errors.Join(err, closeClient()) }()
	seen := map[int64]bool{}
	for id, inputErr := range ids {
		if inputErr != nil {
			return report, inputErr
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		artwork, err := client.Artwork(id).Get(ctx)
		if err != nil {
			return report, err
		}
		value, ok, err := project(artwork, time.Now().UTC())
		if err != nil {
			return report, err
		}
		if !ok {
			return report, i18n.Errorf("error.vector_artwork_unavailable", id)
		}
		if err = s.put(ctx, value); err != nil {
			return report, err
		}
		report.Recorded++
		options.artworkID = strconv.FormatInt(id, 10)
		if err = s.embedPages(ctx, client, options, &report); err != nil {
			return report, err
		}
	}
	return report, nil
}
