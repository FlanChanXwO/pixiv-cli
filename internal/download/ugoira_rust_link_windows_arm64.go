//go:build cgo && windows && arm64

package download

/*
#cgo LDFLAGS: -L${SRCDIR}/ugoira_rs/staticlib/aarch64-pc-windows-msvc -lugoira_rs
*/
import "C"
