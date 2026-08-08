// Package localstate 定义本机私有状态的路径命名空间与最小权限要求。
// 它不包含 Pixiv 协议、MCP 传输或日志字段。
package filesystem

const (
	// AppDataDirName 是本项目所有本机持久状态的唯一根目录名。
	AppDataDirName  = ".pixiv-cli"
	PrivateDirMode  = 0o700
	PrivateFileMode = 0o600
)
