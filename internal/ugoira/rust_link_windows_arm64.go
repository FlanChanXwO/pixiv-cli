//go:build cgo && windows && arm64

package ugoira

/*
#cgo LDFLAGS: -L${SRCDIR}/../download/ugoira_rs/staticlib/aarch64-pc-windows-msvc -lugoira_rs -ladvapi32 -lntdll -luserenv -lws2_32 -ldbghelp
*/
import "C"
