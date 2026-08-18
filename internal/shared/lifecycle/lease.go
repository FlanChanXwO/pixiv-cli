package lifecycle

import "sync"

// Lease 持有一个需要在使用后释放的资源。
//
// Lease 只能通过 NewLease 创建；Close 可以被重复调用，底层释放函数最多
// 执行一次，所有调用都会得到同一个释放错误。
type Lease[T any] struct {
	value T

	closeOnce sync.Once
	closeFn   func() error
	closeErr  error
}

// NewLease 创建一个资源 Lease。closeFn 为 nil 时使用无操作释放函数。
func NewLease[T any](value T, closeFn func() error) *Lease[T] {
	if closeFn == nil {
		closeFn = func() error { return nil }
	}
	return &Lease[T]{
		value:   value,
		closeFn: closeFn,
	}
}

// Value 返回 Lease 持有的资源。nil Lease 返回 T 的零值。
func (l *Lease[T]) Value() (zero T) {
	if l == nil {
		return zero
	}
	return l.value
}

// Close 释放 Lease 持有的资源。
//
// 释放函数在并发或重复调用下最多执行一次；首次调用产生的错误会被后续
// 调用复用。
func (l *Lease[T]) Close() error {
	if l == nil {
		return nil
	}
	l.closeOnce.Do(func() {
		if l.closeFn != nil {
			l.closeErr = l.closeFn()
		}
	})
	return l.closeErr
}
