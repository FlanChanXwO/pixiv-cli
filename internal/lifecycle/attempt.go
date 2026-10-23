package lifecycle

import "sync/atomic"

// Attempt 标记一次资源使用是否已经越过可重放边界。
// state 只允许从 0 到 1，Commit 幂等且不会回退。
type Attempt struct {
	state atomic.Uint32
}

// Commit 标记本次尝试已经提交给调用方或产生外部副作用。
func (a *Attempt) Commit() {
	if a == nil {
		return
	}
	a.state.Store(1)
}

// Committed 返回本次尝试是否已经越过可重放边界。
func (a *Attempt) Committed() bool {
	return a != nil && a.state.Load() == 1
}
