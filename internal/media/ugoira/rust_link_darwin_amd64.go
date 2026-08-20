//go:build cgo && darwin && amd64

package ugoira

/*
#cgo LDFLAGS: ${SRCDIR}/rust/staticlib/x86_64-apple-darwin/libugoira_rs.a
*/
import "C"
