// Package netdiagnostic 从标准库类型提取安全的网络日志属性，不读取错误文本或 URL。
//
// [Attrs] 由协议请求与应用来源读取共用；字段只包含操作、网络、IP 地址与已知 errno。
package netdiagnostic

import (
	"errors"
	"log/slog"
	"net"
	"strconv"
	"syscall"
)

const (
	attrOperation = "op"
	attrNetwork   = "net"
	attrAddress   = "address"
	attrErrno     = "errno"
)

// Attrs 提取操作、网络、远端地址与已知 errno 名。
// 地址只接受标准库 IP 地址类型；未知 errno 不使用 Error 文本代替名称。
func Attrs(err error) []slog.Attr {
	var attrs []slog.Attr
	var operationError *net.OpError
	for cause := err; errors.As(cause, &operationError); cause = operationError.Err {
		if len(attrs) == 0 {
			attrs = append(attrs, slog.String(attrOperation, operationError.Op), slog.String(attrNetwork, operationError.Net))
		}
		var address string
		switch remote := operationError.Addr.(type) {
		case *net.TCPAddr:
			address = net.JoinHostPort(remote.IP.String(), strconv.Itoa(remote.Port))
		case *net.UDPAddr:
			address = net.JoinHostPort(remote.IP.String(), strconv.Itoa(remote.Port))
		}
		if address != "" {
			attrs = append(attrs, slog.String(attrAddress, address))
			break
		}
	}
	var errno syscall.Errno
	if errors.As(err, &errno) {
		if name := errnoNames[errno]; name != "" {
			attrs = append(attrs, slog.String(attrErrno, name))
		}
	}
	return attrs
}

var errnoNames = map[syscall.Errno]string{
	syscall.ECONNREFUSED: "ECONNREFUSED", syscall.ECONNRESET: "ECONNRESET",
	syscall.ETIMEDOUT: "ETIMEDOUT", syscall.EHOSTUNREACH: "EHOSTUNREACH",
	syscall.ENETUNREACH: "ENETUNREACH", syscall.EPIPE: "EPIPE",
	syscall.ECONNABORTED: "ECONNABORTED", syscall.EADDRNOTAVAIL: "EADDRNOTAVAIL",
	syscall.EACCES: "EACCES", syscall.EPERM: "EPERM",
}
