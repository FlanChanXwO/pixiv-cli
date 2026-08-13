package session

import "sync/atomic"

// Attempt 是一次 operation 是否已经越过 replay boundary 的线程安全标记。
// state 只允许从 0 到 1，Commit 幂等且不会回退。
type Attempt struct {
	state atomic.Uint32
}

// Commit 标记本次尝试已提交给调用方或外部副作用。
func (a *Attempt) Commit() {
	if a == nil {
		return
	}
	a.state.Store(1)
}

// Committed 返回本次尝试是否已经越过 replay boundary。
func (a *Attempt) Committed() bool {
	return a != nil && a.state.Load() == 1
}
