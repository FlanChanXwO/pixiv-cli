package pixiv

import "sync"

// downloadProgressResource 是批次调度阶段已知的资源身份与预检结果。
type downloadProgressResource struct {
	base    DownloadProgress
	initial int64
}

type downloadProgressState struct {
	base        DownloadProgress
	transferred int64
	total       int64
	totalKnown  bool
	done        bool
}

// downloadProgressTracker 在锁内汇总并在锁外同步调用用户回调。回调没有缓冲、
// 串行器或恢复逻辑，故仍保留 worker 的真实并发与取消语义。
type downloadProgressTracker struct {
	callback func(DownloadProgress)
	states   []downloadProgressState

	mu               sync.Mutex
	totalTransferred int64
	totalBytes       int64
	totalKnown       bool
	completed        int
}

func newDownloadProgressTracker(callback func(DownloadProgress), resources []downloadProgressResource, totalKnown bool) *downloadProgressTracker {
	if callback == nil {
		return nil
	}
	tracker := &downloadProgressTracker{
		callback:   callback,
		states:     make([]downloadProgressState, len(resources)),
		totalKnown: totalKnown,
	}
	for index, resource := range resources {
		state := downloadProgressState{base: resource.base, transferred: resource.initial, total: resource.base.ResourceTotalBytes, totalKnown: resource.base.ResourceTotalKnown}
		tracker.states[index] = state
		tracker.totalTransferred += state.transferred
		if state.totalKnown {
			tracker.totalBytes += state.total
		}
	}
	return tracker
}

func (t *downloadProgressTracker) start(index int) { t.emit(index, 0, false) }

func (t *downloadProgressTracker) add(index int, bytes int64) {
	if bytes > 0 {
		t.emit(index, bytes, false)
	}
}

func (t *downloadProgressTracker) complete(index int) { t.emit(index, 0, true) }

func (t *downloadProgressTracker) emit(index int, delta int64, completed bool) {
	t.mu.Lock()
	state := &t.states[index]
	if delta > 0 {
		state.transferred += delta
		t.totalTransferred += delta
	}
	if completed && !state.done {
		state.done = true
		t.completed++
		// 条件重验证命中不传输 body，但已验证的目标文件仍然代表完整资源。
		if state.totalKnown && state.transferred < state.total {
			missing := state.total - state.transferred
			state.transferred += missing
			t.totalTransferred += missing
		}
	}
	event := state.base
	event.ResourceBytesTransferred = state.transferred
	event.ResourceTotalBytes = state.total
	event.ResourceTotalKnown = state.totalKnown
	event.TotalBytesTransferred = t.totalTransferred
	event.TotalBytes = t.totalBytes
	event.TotalBytesKnown = t.totalKnown
	event.CompletedResources = t.completed
	event.TotalResources = len(t.states)
	callback := t.callback
	t.mu.Unlock()
	callback(event)
}
