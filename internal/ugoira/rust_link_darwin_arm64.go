//go:build cgo && darwin && arm64

package ugoira

/*
#cgo LDFLAGS: ${SRCDIR}/../downloader/ugoira_rs/staticlib/aarch64-apple-darwin/libugoira_rs.a
*/
import "C"
