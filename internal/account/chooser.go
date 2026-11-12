package account

import (
	"crypto/rand"
	"fmt"
	"math/big"
)

// choose 只根据快照轮流或随机选择；Store 会在提交前再次验证
// 返回 UID 属于候选快照。
func choose(snapshot PoolSnapshot, strategy Strategy) (int64, error) {
	switch strategy {
	case StrategyRoundRobin:
		if snapshot.MarkerSortOrder != nil {
			for _, candidate := range snapshot.Candidates {
				if candidate.SortOrder > *snapshot.MarkerSortOrder {
					return candidate.UserID, nil
				}
			}
		}
		return snapshot.Candidates[0].UserID, nil
	case StrategyRandom:
		index, err := rand.Int(rand.Reader, big.NewInt(int64(len(snapshot.Candidates))))
		if err != nil {
			return 0, err
		}
		return snapshot.Candidates[index.Int64()].UserID, nil
	default:
		return 0, fmt.Errorf("unsupported account pool strategy %q", strategy)
	}
}
