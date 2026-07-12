//go:build cgo && windows && amd64

package download

/*
#cgo LDFLAGS: -L${SRCDIR}/ugoira_rs/staticlib/x86_64-pc-windows-msvc -lugoira_rs
*/
import "C"
