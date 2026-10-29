package pool

import (
	"crypto/rand"
	"fmt"
	"math/big"

	"github.com/FlanChanXwO/pixiv-cli/internal/config/settings"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/account"
)

// choose 在无 IO 的前提下执行 round-robin 或随机选择。Database 会在提交前再次验证
// 返回 UID 属于候选快照。
func choose(snapshot account.PoolSnapshot, strategy settings.AccountPoolStrategy) (int64, error) {
	if len(snapshot.Candidates) == 0 {
		return 0, &account.PoolSelectionError{
			Kind:                account.PoolSelectionExhausted,
			EarliestFrozenUntil: snapshot.EarliestFrozenUntil,
		}
	}
	switch strategy {
	case settings.AccountPoolStrategyRoundRobin:
		if snapshot.MarkerSortOrder != nil {
			for _, candidate := range snapshot.Candidates {
				if candidate.SortOrder > *snapshot.MarkerSortOrder {
					return candidate.UserID, nil
				}
			}
		}
		return snapshot.Candidates[0].UserID, nil
	case settings.AccountPoolStrategyRandom:
		index, err := rand.Int(rand.Reader, big.NewInt(int64(len(snapshot.Candidates))))
		if err != nil {
			return 0, err
		}
		return snapshot.Candidates[index.Int64()].UserID, nil
	default:
		return 0, fmt.Errorf("unsupported account pool strategy %q", strategy)
	}
}
