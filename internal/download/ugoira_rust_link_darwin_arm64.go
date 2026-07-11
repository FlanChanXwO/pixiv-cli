//go:build cgo && darwin && arm64

package download

/*
#cgo LDFLAGS: ${SRCDIR}/ugoira_rs/staticlib/aarch64-apple-darwin/libugoira_rs.a
*/
import "C"
