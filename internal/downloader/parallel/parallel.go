// Package parallel 提供仅依赖 context 与 Go runtime 的有序批处理并发基础设施。
package parallel

import (
	"context"
	"runtime"
	"sync"
)

// SystemWorkerCount 采用当前 GOMAXPROCS；它会尊重容器 CPU quota、运行时环境和
// 调用��对 Go scheduler 的显式设置，不暴露 pixiv-cli 自己的并发配置或硬编码上限。
func SystemWorkerCount() int {
	workers := runtime.GOMAXPROCS(0)
	if workers < 1 {
		return 1
	}
	return workers
}

// WorkerCount 将系统可用并发收敛到本批任务数，既不创建空 worker，也不以固定数字
// 限制用户的下载任务。
func WorkerCount(tasks int) int {
	if tasks <= 0 {
		return 0
	}
	workers := SystemWorkerCount()
	if tasks < workers {
		return tasks
	}
	return workers
}

// ForEach 在当前运行时可用并行度内处理每个索引。work 负责把各索引的业务失败写入
// 自己的结果槽，避免一个作品失败取消同一批的其他作品；context 取消仍立即停止派发。
func ForEach(ctx context.Context, tasks int, work func(context.Context, int)) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	workers := WorkerCount(tasks)
	if workers == 0 {
		return nil
	}
	jobs := make(chan int)
	var group sync.WaitGroup
	group.Add(workers)
	for range workers {
		go func() {
			defer group.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case index, ok := <-jobs:
					if !ok {
						return
					}
					work(ctx, index)
				}
			}
		}()
	}
	for index := range tasks {
		select {
		case <-ctx.Done():
			close(jobs)
			group.Wait()
			return ctx.Err()
		case jobs <- index:
		}
	}
	close(jobs)
	group.Wait()
	return ctx.Err()
}
