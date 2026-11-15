package netdiagnostic

import "golang.org/x/sys/windows"

// Windows socket 返回 WSA errno，syscall.ECONNREFUSED 等为合成值。
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
