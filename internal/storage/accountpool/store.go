package accountpool

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"slices"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/application"
	"github.com/FlanChanXwO/pixiv-cli/internal/config"
	constants "github.com/FlanChanXwO/pixiv-cli/internal/platform/localstate"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/localstate"
	"github.com/FlanChanXwO/pixiv-cli/internal/utils/files"
)

// State 是不含凭据、查询或下载历史的账号池运行状态。
type State struct {
	LastUserID  int64               `json:"last_user_id,omitempty"`
	FrozenUntil map[int64]time.Time `json:"frozen_until,omitempty"`
}

// Store 以独立 JSON 文件保存跨进程账号池状态。Random 仅供测试注入；生产默认
// 使用 crypto/rand 以均匀选择可用账号。
type Store struct {
	Path   func() (string, error)
	Random func(int) (int, error)
}

func DefaultPath() (string, error) {
	return files.UserDataFile(constants.AppDataDirName, "data/account-pool.json")
}

func DefaultStore() Store { return Store{Path: DefaultPath} }

func (s Store) Lease(ctx context.Context, configured, candidates []int64, strategy config.AccountPoolStrategy, now time.Time) (int64, error) {
	if len(configured) == 0 {
		return 0, application.ErrAccountPoolExhausted
	}
	path, err := s.path()
	if err != nil {
		return 0, err
	}
	var selected int64
	err = localstate.WithPrivateLock(ctx, path, func() error {
		state, err := load(path)
		if err != nil {
			return err
		}
		prune(&state, configured, now)
		available := availableCandidates(configured, candidates, state.FrozenUntil, now)
		if len(available) == 0 {
			if err := localstate.WriteJSON(path, state); err != nil {
				return err
			}
			return application.ErrAccountPoolExhausted
		}
		selected, err = s.choose(state.LastUserID, configured, available, strategy)
		if err != nil {
			return err
		}
		state.LastUserID = selected
		return localstate.WriteJSON(path, state)
	})
	return selected, err
}

func (s Store) Freeze(ctx context.Context, userID int64, until, now time.Time) error {
	if userID <= 0 {
		return errors.New("account pool user ID must be positive")
	}
	path, err := s.path()
	if err != nil {
		return err
	}
	return localstate.WithPrivateLock(ctx, path, func() error {
		state, err := load(path)
		if err != nil {
			return err
		}
		if state.FrozenUntil == nil {
			state.FrozenUntil = make(map[int64]time.Time)
		}
		// 不缩短其他进程已记录的更长 Retry-After 冻结，避免并发 429 让账号过早
		// 重回候选集。锁保证读取、比较和写入属于同一个状态事务。
		if previous, exists := state.FrozenUntil[userID]; exists && previous.After(until) {
			until = previous
		}
		if until.After(now) {
			state.FrozenUntil[userID] = until
		} else {
			delete(state.FrozenUntil, userID)
		}
		return localstate.WriteJSON(path, state)
	})
}

func (s Store) path() (string, error) {
	if s.Path == nil {
		return DefaultPath()
	}
	return s.Path()
}

func load(path string) (State, error) {
	state := State{FrozenUntil: make(map[int64]time.Time)}
	_, err := localstate.ReadJSON(path, &state)
	if err != nil {
		return State{}, err
	}
	if state.FrozenUntil == nil {
		state.FrozenUntil = make(map[int64]time.Time)
	}
	return state, nil
}

func prune(state *State, configured []int64, now time.Time) {
	for userID, until := range state.FrozenUntil {
		if !slices.Contains(configured, userID) || !until.After(now) {
			delete(state.FrozenUntil, userID)
		}
	}
	if !slices.Contains(configured, state.LastUserID) {
		state.LastUserID = 0
	}
}

func availableCandidates(configured, candidates []int64, frozen map[int64]time.Time, now time.Time) []int64 {
	available := make([]int64, 0, len(candidates))
	for _, userID := range candidates {
		if !slices.Contains(configured, userID) {
			continue
		}
		if until, frozen := frozen[userID]; frozen && until.After(now) {
			continue
		}
		available = append(available, userID)
	}
	return available
}

func (s Store) choose(lastUserID int64, configured, available []int64, strategy config.AccountPoolStrategy) (int64, error) {
	switch strategy {
	case config.AccountPoolStrategyRoundRobin:
		start := slices.Index(configured, lastUserID)
		for offset := 1; offset <= len(configured); offset++ {
			userID := configured[(start+offset+len(configured))%len(configured)]
			if slices.Contains(available, userID) {
				return userID, nil
			}
		}
		return 0, application.ErrAccountPoolExhausted
	case config.AccountPoolStrategyRandom:
		index, err := s.randomIndex(len(available))
		if err != nil {
			return 0, err
		}
		return available[index], nil
	default:
		return 0, fmt.Errorf("unsupported account pool strategy %q", strategy)
	}
}

func (s Store) randomIndex(size int) (int, error) {
	if size <= 0 {
		return 0, application.ErrAccountPoolExhausted
	}
	if s.Random != nil {
		index, err := s.Random(size)
		if err != nil {
			return 0, err
		}
		if index < 0 || index >= size {
			return 0, errors.New("account pool random source returned an invalid index")
		}
		return index, nil
	}
	value, err := rand.Int(rand.Reader, big.NewInt(int64(size)))
	if err != nil {
		return 0, err
	}
	return int(value.Int64()), nil
}
