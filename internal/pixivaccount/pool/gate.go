package pool

import "context"

// Gate 串行化可能消耗同一 Pixiv refresh token 的执行。它属于产品账号池状态，不放在
// MCP server 全局。
type Gate struct{ slots chan struct{} }

// NewGate 创建一个单槽 gate。
func NewGate() *Gate { return &Gate{slots: make(chan struct{}, 1)} }

// Acquire 等待串行化槽，成功后必须与 Release 配对。
func (g *Gate) Acquire(ctx context.Context) error {
	select {
	case g.slots <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Release 归还 Acquire 取得的槽。
func (g *Gate) Release() {
	<-g.slots
}
