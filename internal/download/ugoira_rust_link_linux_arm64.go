//go:build cgo && linux && arm64

package download

/*
#cgo LDFLAGS: ${SRCDIR}/ugoira_rs/staticlib/aarch64-unknown-linux-gnu/libugoira_rs.a -lm
*/
import "C"
