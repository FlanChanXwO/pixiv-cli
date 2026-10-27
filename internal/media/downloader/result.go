package downloader

// downloadedResourceType 标记没有 artwork 元数据的直接资源产物。
const downloadedResourceType = "resource"

// 拒绝的 source 可能是用户误传的签名直链；失败记录只保留稳定占位符，避免
// CLI/MCP 把原始 locator 回显到 stdout、JSON 或 structured result。
const redactedDownloadSource = "[redacted source]"

// DownloadedArtwork 保存一篇作品成功写出的文件集合，Files 按页码顺序排列。
type DownloadedArtwork struct {
	IllustID int64
	Title    string
	Author   string
	Type     string
	Files    []DownloadedFile
}

// DownloadedFile 保存一个已发布文件的绝对路径与 1-based 页码。
type DownloadedFile struct {
	Path string
	Page int
}

// DownloadFailure 保存一次无状态批量下载中单个作品或作者列表读取的可展示失败。
// Message 仅承接 SDK/下载器的安全错误文本，不保存用户输入的原始 URL。
type DownloadFailure struct {
	URL      string
	IllustID int64
	Type     string
	Message  string
	// Cause 只用于 CLI 判断账号池的安全重试边界，输出 adapter 只公开 Message。
	Cause error
}

// DownloadWarning 保存成功下载结果中需要调用方关注、但不阻止产物发布的提示。
// Message 仅承接安全的本地分类文本；输出 adapter 可以据此展示 warning。
type DownloadWarning struct {
	IllustID int64
	Type     string
	Message  string
}

// DownloadReport 同时保留成功产物和独立失败，使作者全量下载无需持久化 job 状态也能说明结果。
type DownloadReport struct {
	Items     []DownloadedArtwork
	Failures  []DownloadFailure
	Warnings  []DownloadWarning
	Committed bool
}
