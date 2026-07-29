//go:build cgo && linux && amd64

package ugoira

/*
#cgo LDFLAGS: ${SRCDIR}/../download/ugoira_rs/staticlib/x86_64-unknown-linux-gnu/libugoira_rs.a -lm
*/
import "C"
