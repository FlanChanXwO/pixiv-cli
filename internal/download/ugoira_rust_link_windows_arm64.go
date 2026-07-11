//go:build cgo && windows && arm64

package download

/*
#cgo LDFLAGS: ${SRCDIR}/ugoira_rs/staticlib/aarch64-pc-windows-msvc/ugoira_rs.lib
*/
import "C"
