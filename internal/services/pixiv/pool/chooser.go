package pool

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"

	config "github.com/FlanChanXwO/pixiv-cli/internal/config/settings"
	accountpixiv "github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/account"
)

// Choose 是无 IO 的 round-robin/random chooser。Database 会在提交前再次验证
// 返回 UID 属于候选快照。
func Choose(snapshot accountpixiv.PoolSnapshot, strategy config.AccountPoolStrategy, random func(int) (int, error)) (int64, error) {
	if len(snapshot.Candidates) == 0 {
		return 0, &accountpixiv.PoolSelectionError{
			Kind:                accountpixiv.PoolSelectionExhausted,
			EarliestFrozenUntil: cloneInt64(snapshot.EarliestFrozenUntil),
		}
	}
	switch strategy {
	case config.AccountPoolStrategyRoundRobin:
		if snapshot.MarkerSortOrder != nil {
			for _, candidate := range snapshot.Candidates {
				if candidate.SortOrder > *snapshot.MarkerSortOrder {
					return candidate.UserID, nil
				}
			}
		}
		return snapshot.Candidates[0].UserID, nil
	case config.AccountPoolStrategyRandom:
		if random == nil {
			random = randomIndex
		}
		index, err := random(len(snapshot.Candidates))
		if err != nil {
			return 0, err
		}
		if index < 0 || index >= len(snapshot.Candidates) {
			return 0, errors.New("pixiv account pool random source returned an invalid index")
		}
		return snapshot.Candidates[index].UserID, nil
	default:
		return 0, fmt.Errorf("unsupported account pool strategy %q", strategy)
	}
}

func randomIndex(size int) (int, error) {
	if size <= 0 {
		return 0, errors.New("pixiv account pool has no eligible account")
	}
	value, err := rand.Int(rand.Reader, big.NewInt(int64(size)))
	if err != nil {
		return 0, err
	}
	return int(value.Int64()), nil
}

func cloneInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	out := *value
	return &out
}
