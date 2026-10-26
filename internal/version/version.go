// Package version exposes metadata embedded by the Go linker at build time.
package version

// Version 由发布构建通过 -ldflags -X 覆盖；本地开发构建保留可识别的默认值。
var Version = "dev"

// Info 是 CLI 可安全展示的构建元数据。
type Info struct {
	Version string `json:"version"`
}

// Current 返回当前二进制嵌入的构建元数据。
func Current() Info {
	return Info{
		Version: Version,
	}
}

// IsDevelopment 识别没有发布渠道的开发构建。
func (info Info) IsDevelopment() bool {
	return info.Version == "dev"
}
