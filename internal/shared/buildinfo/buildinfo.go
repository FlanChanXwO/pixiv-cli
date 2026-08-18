// Package buildinfo exposes metadata embedded by the Go linker at build time.
package buildinfo

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

// IsDevelopment 仅根据版本号判断开发构建，供更新功能在未来拒绝 dev 二进制时使用。
func (info Info) IsDevelopment() bool {
	return info.Version == "dev"
}
