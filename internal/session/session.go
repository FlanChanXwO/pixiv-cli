// Package session 提供不包含账号、重试或 HTTP policy 的通用生命周期。
package session

import (
	"context"
	"errors"
)

// Open 打开一次 operation resource。open 失败时，Run 不会调用 close。
type Open[T any] func(context.Context) (value T, close func() error, err error)

// Use 使用一次已打开的 resource。Attempt 是 replay boundary 的唯一状态。
type Use[T any] func(context.Context, T, *Attempt) error

// Run 执行一次完整生命周期：创建 child context/Attempt、open、use，再恰好
// 调用一次 close；use 与 close 的错误都会保留。
func Run[T any](ctx context.Context, open Open[T], use Use[T]) error {
	if ctx == nil {
		return errors.New("session: context is nil")
	}
	if open == nil {
		return errors.New("session: open function is nil")
	}
	if use == nil {
		return errors.New("session: use function is nil")
	}
	child, cancel := context.WithCancel(ctx)
	defer cancel()
	attempt := &Attempt{}
	value, closeResource, err := open(child)
	if err != nil {
		return err
	}
	if closeResource == nil {
		closeResource = func() error { return nil }
	}
	useErr := use(child, value, attempt)
	closeErr := closeResource()
	return errors.Join(useErr, closeErr)
}
