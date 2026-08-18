// Package lifecycle 提供与产品、账号和传输协议无关的资源生命周期。
package lifecycle

import (
	"context"
	"errors"
)

var (
	// ErrNilContext 表示调用方没有提供 context。
	ErrNilContext = errors.New("lifecycle: context is nil")
	// ErrNilOpen 表示调用方没有提供资源打开函数。
	ErrNilOpen = errors.New("lifecycle: open function is nil")
	// ErrNilUse 表示调用方没有提供资源使用函数。
	ErrNilUse = errors.New("lifecycle: use function is nil")
	// ErrNilLease 表示打开函数成功但没有返回 Lease。
	ErrNilLease = errors.New("lifecycle: open function returned a nil lease")
)

// Open 打开一次资源。若打开失败但仍返回非 nil Lease，Run 仍会负责关闭它。
type Open[T any] func(context.Context) (*Lease[T], error)

// Use 使用一次已经打开的资源。Attempt 是可重放边界的唯一状态。
type Use[T any] func(context.Context, T, *Attempt) error

// Run 执行一次完整生命周期：创建 child context 和 Attempt，打开资源，
// 使用资源，再恰好调用一次 Lease.Close；use 与 close 的错误都会保留。
// 即使 use 发生 panic，也会先关闭 Lease，再继续传播 panic。
func Run[T any](ctx context.Context, open Open[T], use Use[T]) (err error) {
	if ctx == nil {
		return ErrNilContext
	}
	if open == nil {
		return ErrNilOpen
	}
	if use == nil {
		return ErrNilUse
	}

	child, cancel := context.WithCancel(ctx)
	defer cancel()

	lease, err := open(child)
	if err != nil {
		if lease == nil {
			return err
		}
		return errors.Join(err, lease.Close())
	}
	if lease == nil {
		return ErrNilLease
	}

	defer func() {
		closeErr := lease.Close()
		if closeErr != nil {
			err = errors.Join(err, closeErr)
		}
	}()

	err = use(child, lease.Value(), &Attempt{})
	return err
}
