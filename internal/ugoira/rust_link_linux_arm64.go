//go:build cgo && linux && arm64

package ugoira

/*
#cgo LDFLAGS: ${SRCDIR}/../download/ugoira_rs/staticlib/aarch64-unknown-linux-gnu/libugoira_rs.a -lm
*/
import "C"
