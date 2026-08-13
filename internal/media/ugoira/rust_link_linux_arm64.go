//go:build cgo && linux && arm64

package ugoira

/*
#cgo LDFLAGS: ${SRCDIR}/rust/staticlib/aarch64-unknown-linux-gnu/libugoira_rs.a -lm
*/
import "C"
