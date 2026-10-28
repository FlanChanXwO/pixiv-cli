// transport.go 从标准库 typed cause 提取安全的传输诊断，绝不读取错误文本或 URL。
package logging

import (
	"errors"
	"log/slog"
	"net"
	"strconv"
	"syscall"
)

// AttrNetworkOperation 等键标记 typed 传输原因的安全字段。
const (
	AttrNetworkOperation = "op"
	AttrNetwork          = "net"
	AttrAddress          = "address"
	AttrErrno            = "errno"
)

// TransportAttrs 提取操作、网络、远端地址与已知 errno 名。
// 地址只接受标准库 IP 地址类型；未知 errno 不使用 Error 文本代替名称。
func TransportAttrs(err error) []slog.Attr {
	var attrs []slog.Attr
	var operationError *net.OpError
	for cause := err; errors.As(cause, &operationError); cause = operationError.Err {
		if len(attrs) == 0 {
			attrs = append(attrs, slog.String(AttrNetworkOperation, operationError.Op), slog.String(AttrNetwork, operationError.Net))
		}
		var address string
		switch remote := operationError.Addr.(type) {
		case *net.TCPAddr:
			address = net.JoinHostPort(remote.IP.String(), strconv.Itoa(remote.Port))
		case *net.UDPAddr:
			address = net.JoinHostPort(remote.IP.String(), strconv.Itoa(remote.Port))
		}
		if address != "" {
			attrs = append(attrs, slog.String(AttrAddress, address))
			break
		}
	}
	var errno syscall.Errno
	if errors.As(err, &errno) {
		names := map[syscall.Errno]string{
			syscall.ECONNREFUSED: "ECONNREFUSED", syscall.ECONNRESET: "ECONNRESET",
			syscall.ETIMEDOUT: "ETIMEDOUT", syscall.EHOSTUNREACH: "EHOSTUNREACH",
			syscall.ENETUNREACH: "ENETUNREACH", syscall.EPIPE: "EPIPE",
			syscall.ECONNABORTED: "ECONNABORTED", syscall.EADDRNOTAVAIL: "EADDRNOTAVAIL",
			syscall.EACCES: "EACCES", syscall.EPERM: "EPERM",
		}
		if name := names[errno]; name != "" {
			attrs = append(attrs, slog.String(AttrErrno, name))
		}
	}
	return attrs
}
