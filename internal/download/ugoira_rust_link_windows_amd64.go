//go:build cgo && windows && amd64

package download

/*
#cgo LDFLAGS: -L${SRCDIR}/ugoira_rs/staticlib/x86_64-pc-windows-msvc -lugoira_rs -ladvapi32 -lntdll -luserenv -lws2_32 -ldbghelp
*/
import "C"

// Rust staticlib 不会向 cgo 传播 std 的 Windows native import-library 依赖；
// 这里与 archive 一起显式传递，避免真实 MSVC/LLD 链接时遗漏 NT、网络和诊断符号。
