//go:build cgo && darwin && arm64

package ugoira

/*
#cgo LDFLAGS: ${SRCDIR}/rust/staticlib/aarch64-apple-darwin/libugoira_rs.a
*/
import "C"
