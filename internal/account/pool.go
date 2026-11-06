package account

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"
)

// Strategy 确定每次操作在合格账号中采用的选择顺序。
type Strategy string

const (
	// StrategyRoundRobin 按显示顺序轮流选择。
	StrategyRoundRobin Strategy = "round_robin"
	// StrategyRandom 从合格账号中随机选择。
	StrategyRandom Strategy = "random"
)

// SelectionMode 区分调用方固定的账号与交给池调度的操作。
type SelectionMode string

const (
	// SelectionFixed 固定使用 UserID，不因限流而切换账号。
	SelectionFixed SelectionMode = "fixed"
	// SelectionPool 交给池选择并按安全重放规则切换账号。
	SelectionPool SelectionMode = "pool"
)

// Selection 由调用方解析默认配置后给出；固定模式的 UserID 必须为正数。
type Selection struct {
	Mode   SelectionMode
	UserID int64
}

// PoolCandidate 提供不含凭据的调度快照；SortOrder 是当前数组中的显示序号。
type PoolCandidate struct {
	UserID           int64
	SortOrder        int64
	Schedulable      bool
	PoolFrozenUntil  *int64
	PoolLastSelected bool
	Eligible         bool
}

// PoolSnapshot 向纯选择函数提供候选与上次选择位置。
type PoolSnapshot struct {
	Candidates          []PoolCandidate
	MarkerUserID        *int64
	MarkerSortOrder     *int64
	EarliestFrozenUntil *int64
}

// Chooser 在持锁的快照内选择一个候选 UID，不执行 I/O。
type Chooser func(PoolSnapshot) (int64, error)

// PoolStatus 包含所有账号的池状态；时间使用 Unix 秒以保持现有命令输出。
type PoolStatus struct {
	Accounts            []PoolCandidate
	EarliestFrozenUntil *int64
}

// PoolSelectionKind 区分候选不足的实际原因。
type PoolSelectionKind string

const (
	// PoolSelectionNoLocalAccount 表示本地尚无账号。
	PoolSelectionNoLocalAccount PoolSelectionKind = "no_local_account"
	// PoolSelectionNoSchedulable 表示没有启用的池成员。
	PoolSelectionNoSchedulable PoolSelectionKind = "no_schedulable_account"
	// PoolSelectionAllFrozen 表示全部池成员仍在冻结。
	PoolSelectionAllFrozen PoolSelectionKind = "all_frozen"
	// PoolSelectionExhausted 表示本次操作已用尽合格候选。
	PoolSelectionExhausted PoolSelectionKind = "exhausted"
)

// PoolSelectionError 携带无可用账号的原因与最早解冻时间。
type PoolSelectionError struct {
	Kind                PoolSelectionKind
	EarliestFrozenUntil *int64
}

// Error 返回池选择失败的原因。
func (e *PoolSelectionError) Error() string {
	return "pixiv account pool selection failed: " + string(e.Kind)
}

// SetPixivSchedulable 原子修改一组账号的池成员资格；任一 UID 不存在时整批保留原值。
func (s *Store) SetPixivSchedulable(ctx context.Context, userIDs []int64, enabled bool) error {
	if len(userIDs) == 0 {
		return errors.New("account: account pool requires at least one user id")
	}
	return s.update(ctx, func(doc *document) error {
		seen := make(map[int64]bool, len(userIDs))
		now := time.Now().UTC()
		for _, id := range userIDs {
			if id <= 0 || seen[id] {
				return errors.New("account: invalid or duplicate pool user id")
			}
			seen[id] = true
			index := slices.IndexFunc(doc.Pixiv, func(a Pixiv) bool { return a.UserID == id })
			if index < 0 {
				return ErrPixivNotFound
			}
			doc.Pixiv[index].Pool.Enabled = enabled
			doc.Pixiv[index].UpdatedAt = now
		}
		return nil
	})
}

// SetAllPixivSchedulable 在一个事务中修改全部账号的池成员资格。
func (s *Store) SetAllPixivSchedulable(ctx context.Context, enabled bool) error {
	return s.update(ctx, func(doc *document) error {
		now := time.Now().UTC()
		for i := range doc.Pixiv {
			doc.Pixiv[i].Pool.Enabled = enabled
			doc.Pixiv[i].UpdatedAt = now
		}
		return nil
	})
}

func (d *document) poolStatus(now int64) PoolStatus {
	status := PoolStatus{Accounts: make([]PoolCandidate, 0, len(d.Pixiv))}
	for i := range d.Pixiv {
		a := &d.Pixiv[i]
		if a.Pool.FrozenUntil != nil && a.Pool.FrozenUntil.Unix() <= now {
			a.Pool.FrozenUntil = nil
		}
		candidate := PoolCandidate{UserID: a.UserID, SortOrder: int64(i + 1), Schedulable: a.Pool.Enabled, PoolLastSelected: a.UserID == d.LastUserID, Eligible: a.Pool.Enabled && a.Pool.FrozenUntil == nil}
		if a.Pool.FrozenUntil != nil {
			value := a.Pool.FrozenUntil.Unix()
			candidate.PoolFrozenUntil = &value
			if status.EarliestFrozenUntil == nil || value < *status.EarliestFrozenUntil {
				status.EarliestFrozenUntil = &value
			}
		}
		status.Accounts = append(status.Accounts, candidate)
	}
	return status
}

// ListPixivPoolStatus 在同一事务内清理过期冻结并返回完整调度状态。
func (s *Store) ListPixivPoolStatus(ctx context.Context, now int64) (PoolStatus, error) {
	var status PoolStatus
	err := s.update(ctx, func(doc *document) error { status = doc.poolStatus(now); return nil })
	return status, err
}

// SelectPixiv 在文件锁内选择账号、清理过期冻结并提交 marker；选择失败时整批回滚。
func (s *Store) SelectPixiv(ctx context.Context, now int64, attempted []int64, choose Chooser) (Pixiv, error) {
	var selected Pixiv
	err := s.update(ctx, func(doc *document) error {
		if len(doc.Pixiv) == 0 {
			return &PoolSelectionError{Kind: PoolSelectionNoLocalAccount}
		}
		status := doc.poolStatus(now)
		snapshot := PoolSnapshot{Candidates: []PoolCandidate{}}
		enabled, eligible := 0, 0
		for _, a := range status.Accounts {
			if a.PoolLastSelected {
				id, order := a.UserID, a.SortOrder
				snapshot.MarkerUserID = &id
				snapshot.MarkerSortOrder = &order
			}
			if !a.Schedulable {
				continue
			}
			enabled++
			if a.PoolFrozenUntil != nil && (snapshot.EarliestFrozenUntil == nil || *a.PoolFrozenUntil < *snapshot.EarliestFrozenUntil) {
				snapshot.EarliestFrozenUntil = a.PoolFrozenUntil
			}
			if a.Eligible {
				eligible++
				if !slices.Contains(attempted, a.UserID) {
					snapshot.Candidates = append(snapshot.Candidates, a)
				}
			}
		}
		if enabled == 0 {
			return &PoolSelectionError{Kind: PoolSelectionNoSchedulable}
		}
		if eligible == 0 {
			return &PoolSelectionError{Kind: PoolSelectionAllFrozen, EarliestFrozenUntil: snapshot.EarliestFrozenUntil}
		}
		if len(snapshot.Candidates) == 0 {
			return &PoolSelectionError{Kind: PoolSelectionExhausted, EarliestFrozenUntil: snapshot.EarliestFrozenUntil}
		}
		id, err := choose(snapshot)
		if err != nil {
			return err
		}
		if !slices.ContainsFunc(snapshot.Candidates, func(a PoolCandidate) bool { return a.UserID == id }) {
			return fmt.Errorf("account: pool chooser selected uid %d outside transaction snapshot", id)
		}
		for i := range doc.Pixiv {
			if doc.Pixiv[i].UserID == id {
				doc.Pixiv[i].UpdatedAt = time.Unix(now, 0).UTC()
				selected = doc.Pixiv[i]
				selected.PoolLastSelected = true
				doc.LastUserID = id
				break
			}
		}
		return nil
	})
	return selected, err
}

// Freeze 只延长限流冻结截止时间，保持较长的既有冻结。
func (s *Store) Freeze(ctx context.Context, userID, frozenUntil int64) error {
	return s.update(ctx, func(doc *document) error {
		for i := range doc.Pixiv {
			a := &doc.Pixiv[i]
			if a.UserID != userID {
				continue
			}
			if a.Pool.FrozenUntil == nil || a.Pool.FrozenUntil.Unix() < frozenUntil {
				value := time.Unix(frozenUntil, 0).UTC()
				a.Pool.FrozenUntil = &value
			}
			a.UpdatedAt = time.Now().UTC()
			return nil
		}
		return ErrPixivNotFound
	})
}
