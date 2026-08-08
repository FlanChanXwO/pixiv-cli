//go:build cgo && windows && amd64

package ugoira

/*
#cgo LDFLAGS: -L${SRCDIR}/../downloader/ugoira_rs/staticlib/x86_64-pc-windows-msvc -lugoira_rs -ladvapi32 -lntdll -luserenv -lws2_32 -ldbghelp
*/
import "C"

// Rust staticlib 通过 -L/-l 传给 cgo；直接传 .lib 会被 Windows cgo linker 拒绝。
