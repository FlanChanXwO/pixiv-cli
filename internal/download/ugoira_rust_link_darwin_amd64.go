//go:build cgo && darwin && amd64

package download

/*
#cgo LDFLAGS: ${SRCDIR}/ugoira_rs/staticlib/x86_64-apple-darwin/libugoira_rs.a
*/
import "C"
