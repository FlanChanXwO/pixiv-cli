//go:build cgo && windows && arm64

package ugoira

/*
#cgo LDFLAGS: -L${SRCDIR}/rust/staticlib/aarch64-pc-windows-msvc -lugoira_rs -ladvapi32 -lntdll -luserenv -lws2_32 -ldbghelp
*/
import "C"

// Rust staticlib 通过 -L/-l 传给 cgo；直接传 .lib 会被 Windows cgo linker 拒绝。
