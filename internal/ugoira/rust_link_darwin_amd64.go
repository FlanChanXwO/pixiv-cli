//go:build cgo && darwin && amd64

package ugoira

/*
#cgo LDFLAGS: ${SRCDIR}/../downloader/ugoira_rs/staticlib/x86_64-apple-darwin/libugoira_rs.a
*/
import "C"
