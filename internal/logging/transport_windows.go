// transport_windows.go 补齐 Windows socket 返回的 WSA errno；syscall.ECONNREFUSED 等为合成值。
package logging

import "golang.org/x/sys/windows"

func init() {
	errnoNames[windows.WSAECONNREFUSED] = "WSAECONNREFUSED"
	errnoNames[windows.WSAECONNRESET] = "WSAECONNRESET"
	errnoNames[windows.WSAETIMEDOUT] = "WSAETIMEDOUT"
	errnoNames[windows.WSAEHOSTUNREACH] = "WSAEHOSTUNREACH"
	errnoNames[windows.WSAENETUNREACH] = "WSAENETUNREACH"
	errnoNames[windows.WSAECONNABORTED] = "WSAECONNABORTED"
	errnoNames[windows.WSAEADDRNOTAVAIL] = "WSAEADDRNOTAVAIL"
	errnoNames[windows.WSAEACCES] = "WSAEACCES"
}
