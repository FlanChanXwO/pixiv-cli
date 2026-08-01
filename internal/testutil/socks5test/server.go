// Package socks5test 提供仅供测试使用的最小 SOCKS5 CONNECT server。
package socks5test

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
)

// Server 是无认证 SOCKS5 CONNECT 测试服务。它只实现本项目代理契约需要的
// CONNECT，收到其他命令会明确拒绝，避免测试 helper 意外成为通用代理实现。
type Server struct {
	listener    net.Listener
	mu          sync.Mutex
	connections map[net.Conn]struct{}
	wg          sync.WaitGroup
}

// New 启动监听在 loopback 随机端口上的测试服务。
func New() (*Server, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	server := &Server{listener: listener, connections: make(map[net.Conn]struct{})}
	server.wg.Add(1)
	go func() {
		defer server.wg.Done()
		for {
			connection, err := listener.Accept()
			if err != nil {
				return
			}
			server.mu.Lock()
			server.connections[connection] = struct{}{}
			server.mu.Unlock()
			server.wg.Add(1)
			go func() {
				defer server.wg.Done()
				defer func() {
					server.mu.Lock()
					delete(server.connections, connection)
					server.mu.Unlock()
				}()
				defer connection.Close()
				_ = serve(connection)
			}()
		}
	}()
	return server, nil
}

// URL 返回可传给 HTTP transport 的 socks5 或 socks5h URI。
func (s *Server) URL(scheme string) string {
	return scheme + "://" + s.listener.Addr().String()
}

// Close 等待已开始的转发协程退出。调用方应先取消请求或等其返回。
func (s *Server) Close() error {
	err := s.listener.Close()
	s.mu.Lock()
	for connection := range s.connections {
		_ = connection.Close()
	}
	s.mu.Unlock()
	s.wg.Wait()
	return err
}

func serve(connection net.Conn) error {
	var greeting [2]byte
	if _, err := io.ReadFull(connection, greeting[:]); err != nil {
		return err
	}
	if greeting[0] != 5 {
		return fmt.Errorf("unsupported SOCKS version")
	}
	methods := make([]byte, greeting[1])
	if _, err := io.ReadFull(connection, methods); err != nil {
		return err
	}
	if _, err := connection.Write([]byte{5, 0}); err != nil {
		return err
	}
	request := make([]byte, 4)
	if _, err := io.ReadFull(connection, request); err != nil {
		return err
	}
	if request[0] != 5 || request[1] != 1 || request[2] != 0 {
		return writeFailure(connection, 7)
	}
	host, err := readAddress(connection, request[3])
	if err != nil {
		return writeFailure(connection, 8)
	}
	var port [2]byte
	if _, err := io.ReadFull(connection, port[:]); err != nil {
		return err
	}
	target, err := (&net.Dialer{}).DialContext(context.Background(), "tcp", net.JoinHostPort(host, strconv.Itoa(int(binary.BigEndian.Uint16(port[:])))))
	if err != nil {
		return writeFailure(connection, 5)
	}
	defer target.Close()
	if _, err := connection.Write([]byte{5, 0, 0, 1, 0, 0, 0, 0, 0, 0}); err != nil {
		return err
	}
	errCh := make(chan error, 2)
	go func() { _, err := io.Copy(target, connection); errCh <- err }()
	go func() { _, err := io.Copy(connection, target); errCh <- err }()
	<-errCh
	return nil
}

func readAddress(connection net.Conn, addressType byte) (string, error) {
	switch addressType {
	case 1:
		address := make([]byte, net.IPv4len)
		if _, err := io.ReadFull(connection, address); err != nil {
			return "", err
		}
		return net.IP(address).String(), nil
	case 3:
		var length [1]byte
		if _, err := io.ReadFull(connection, length[:]); err != nil {
			return "", err
		}
		address := make([]byte, length[0])
		if _, err := io.ReadFull(connection, address); err != nil {
			return "", err
		}
		return string(address), nil
	case 4:
		address := make([]byte, net.IPv6len)
		if _, err := io.ReadFull(connection, address); err != nil {
			return "", err
		}
		return net.IP(address).String(), nil
	default:
		return "", fmt.Errorf("unsupported SOCKS address type")
	}
}

func writeFailure(connection net.Conn, code byte) error {
	_, err := connection.Write([]byte{5, code, 0, 1, 0, 0, 0, 0, 0, 0})
	return err
}
