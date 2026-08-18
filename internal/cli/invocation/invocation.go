// Package invocation 保存一次 CLI 调用的标准流。
//
// 该包只保存调用方提供的值，不创建或缓存服务、资源或协议适配器；因此可以被
// root 与 command owner 共同使用而不引入 config、storage、SDK、MCP、update 或 Cobra。
package invocation

import "io"

// Streams 保存调用方提供的标准输入、标准输出和错误输出。
type Streams struct {
	In  io.Reader
	Out io.Writer
	Err io.Writer
}

// NewStreams 构造一次调用的标准流，不替换或包装调用方对象。
func NewStreams(in io.Reader, out, errOut io.Writer) *Streams {
	return &Streams{In: in, Out: out, Err: errOut}
}
