//go:build cgo && windows && amd64

package download

/*
#cgo LDFLAGS: ${SRCDIR}/ugoira_rs/staticlib/x86_64-pc-windows-msvc/ugoira_rs.lib
*/
import "C"
