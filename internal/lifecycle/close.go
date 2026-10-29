package lifecycle

import (
	"errors"
	"sync"
)

// CloseState 登记一次 CLI 执行的逆序关闭函数，不缓存服务或暴露依赖图。
//
// 契约：
//   - 所有权：谁打开资源，谁调用 Add 登记关闭函数；资源在登记前不得被其他人假定已可用。
//   - 关闭顺序：Close 按登记的**逆序**关闭（后打开的先关闭），符合依赖关系。
//   - 幂等：Close 只真正执行一次（sync.Once），重复调用返回同一结果。
//   - 错误聚合：关闭错误全部并入同一个 error，不因第一个失败而漏关其余资源。
//   - 生命周期：调用方须先停止资源生产者、完成所有 Add，再调用 Close；不得在关闭开始后再登记。
//   - 锁只保护登记表，不协调生产者退出；closer 在持锁期间执行，不能对同一 scope 调用 Add 或 Close。
type CloseState struct {
	mu      sync.Mutex
	closers []func() error
	err     error
	once    sync.Once
}

// Add 登记一个关闭函数；调用点只在确实打开资源时登记。
func (s *CloseState) Add(closer func() error) {
	s.mu.Lock()
	s.closers = append(s.closers, closer)
	s.mu.Unlock()
}

// Close 在生产者退出、登记完成后逆序关闭资源并返回聚合错误；它是幂等的。
func (s *CloseState) Close() error {
	s.once.Do(func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		for index := len(s.closers) - 1; index >= 0; index-- {
			s.err = errors.Join(s.err, s.closers[index]())
		}
	})
	return s.err
}
