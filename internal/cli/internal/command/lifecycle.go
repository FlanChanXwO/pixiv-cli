// Package command 提供所有 CLI command owner 共用的无状态生命周期声明。
package command

import "github.com/spf13/cobra"

// LifecycleAnnotationKey 是 command owner 与 composition root 之间的生命周期契约。
// 声明存放在 Cobra command 本身，避免进程级 command-pointer registry 泄漏状态。
const LifecycleAnnotationKey = "pixiv-cli.lifecycle"

// OutputNDJSONAnnotationKey 记录命令本次执行是否把 NDJSON 写到 stdout；root 据此
// 把下游关闭管道造成的 EPIPE 视为正常结束。
const OutputNDJSONAnnotationKey = "pixiv-cli.output-ndjson"

// Execution 描述 root 在命令执行前后负责的通用生命周期行为。业务资源由各
// command owner 的窄依赖端口按需创建，不在这里声明或定位。
type Execution struct {
	StartupHooks    bool
	EnsureConfig    bool
	AutomaticUpdate bool
	MCP             bool
}

// Bind 把静态生命周期声明写入 command 自身。
func Bind(command *cobra.Command, execution Execution) {
	if command.Annotations == nil {
		command.Annotations = make(map[string]string)
	}
	command.Annotations[LifecycleAnnotationKey] = encodeExecution(execution)
}

// For 返回最近的 owner 声明；未声明命令保留 startup 默认行为。
func For(command *cobra.Command) Execution {
	for current := command; current != nil; current = current.Parent() {
		if encoded, ok := current.Annotations[LifecycleAnnotationKey]; ok {
			return decodeExecution(encoded)
		}
	}
	return Execution{StartupHooks: true}
}

// Normal 返回普通成功 CLI leaf 的共同生命周期。
func Normal() Execution {
	return Execution{StartupHooks: true, EnsureConfig: true, AutomaticUpdate: true}
}

func AuthExport() Execution { return Execution{} }

func HiddenCallback() Execution { return Execution{} }

func AuthAccount() Execution { return Normal() }

func AuthLogin() Execution { return Normal() }

func AuthBundleImport() Execution {
	result := AuthAccount()
	result.AutomaticUpdate = false
	return result
}

func ConfigPath() Execution {
	return Execution{StartupHooks: true, EnsureConfig: true, AutomaticUpdate: true}
}

func UpdateCommand() Execution {
	return Execution{StartupHooks: true, EnsureConfig: true}
}

func PixivData() Execution { return Normal() }

func DownloadCommand() Execution { return Normal() }

func FanboxData() Execution { return Normal() }

func FanboxAuth() Execution {
	return Execution{StartupHooks: true, AutomaticUpdate: true}
}

func PixivMCP() Execution {
	return Execution{StartupHooks: true, EnsureConfig: true, MCP: true}
}

func encodeExecution(execution Execution) string {
	encoded := []byte("0000")
	values := [...]bool{
		execution.StartupHooks,
		execution.EnsureConfig,
		execution.AutomaticUpdate,
		execution.MCP,
	}
	for index, value := range values {
		if value {
			encoded[index] = '1'
		}
	}
	return string(encoded)
}

func decodeExecution(encoded string) Execution {
	if len(encoded) != 4 {
		panic("invalid CLI lifecycle annotation")
	}
	for index := range encoded {
		if encoded[index] != '0' && encoded[index] != '1' {
			panic("invalid CLI lifecycle annotation")
		}
	}
	return Execution{
		StartupHooks:    encoded[0] == '1',
		EnsureConfig:    encoded[1] == '1',
		AutomaticUpdate: encoded[2] == '1',
		MCP:             encoded[3] == '1',
	}
}
