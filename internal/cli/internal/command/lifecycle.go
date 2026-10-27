// Package command 提供 CLI 各 command owner 共用的执行环境、生命周期声明、逻辑分页和输出暂存。
//
// 包的组成：[Env] 承载输入输出与 flag 绑定。
// [Execution] 声明启动、更新和 MCP 等执行要求，[Plan] 表示逻辑分页，
// [JSONArraySpool] 暂存单数组 JSON。
package command

import "github.com/spf13/cobra"

// LifecycleAnnotationKey 存放 command owner 与 composition root 之间的生命周期契约。
// 声明存放在 Cobra command 本身，避免进程级 command-pointer registry 泄漏状态。
const LifecycleAnnotationKey = "pixiv-cli.lifecycle"

// OutputNDJSONAnnotationKey 记录命令本次执行是否把 NDJSON 写到 stdout；root 据此
// 把下游关闭管道造成的 EPIPE 视为正常结束。
const OutputNDJSONAnnotationKey = "pixiv-cli.output-ndjson"

// SetOutputNDJSON 记录本次执行是否向 stdout 写标准 NDJSON；root 据此把下游关闭管道
// 导致的 EPIPE 判为正常结束。
func SetOutputNDJSON(cmd *cobra.Command, enabled bool) {
	if cmd.Annotations == nil {
		cmd.Annotations = make(map[string]string)
	}
	if enabled {
		cmd.Annotations[OutputNDJSONAnnotationKey] = "true"
		return
	}
	cmd.Annotations[OutputNDJSONAnnotationKey] = "false"
}

// OutputNDJSON 读取 [SetOutputNDJSON] 记录的结果；未记录时返回 false。
func OutputNDJSON(cmd *cobra.Command) bool {
	return cmd.Annotations[OutputNDJSONAnnotationKey] == "true"
}

// offlineAnnotationKey 标记本次执行不运行自动更新检查。auth 侧写入，update 侧读取；
// 含凭据的输入分类成功后不再发起额外网络请求。
const offlineAnnotationKey = "pixiv-cli.offline"

// SetOffline 让本次执行跳过自动更新检查，供会消费凭据或产生副作用的命令使用。
func SetOffline(cmd *cobra.Command) {
	if cmd.Annotations == nil {
		cmd.Annotations = make(map[string]string)
	}
	cmd.Annotations[offlineAnnotationKey] = "true"
}

// IsOffline 报告本次执行是否调用过 [SetOffline]。
func IsOffline(cmd *cobra.Command) bool {
	return cmd.Annotations[offlineAnnotationKey] == "true"
}

// Execution 描述 root 在命令执行前后负责的通用生命周期行为；业务资源由各
// command owner 的窄依赖端口按需创建，不经过这里定位。
type Execution struct {
	// StartupHooks 启用启动阶段的系统副作用：清理待安装更新和 pixiv:// handler。
	StartupHooks bool
	// EnsureConfig 确保默认配置文件存在，并在配置允许时启动日志。
	EnsureConfig bool
	// AutomaticUpdate 在命令成功后检查更新。
	AutomaticUpdate bool
	// MCP 启用 MCP transport 专用的 broken-pipe 策略。
	MCP bool
}

// Bind 把生命周期声明写进 command 的 annotation，子命令继承时逐级向上查找。
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

// Normal 返回普通 CLI 叶子命令的生命周期：启动钩子、确保配置并检查自动更新。
func Normal() Execution {
	return Execution{StartupHooks: true, EnsureConfig: true, AutomaticUpdate: true}
}

// AuthExport 关闭全部钩子：导出只写文档中约定的 stdout，不能混入启动或更新输出。
func AuthExport() Execution { return Execution{} }

// HiddenCallback 关闭全部钩子；系统协议回调必须只处理本次 URL。
func HiddenCallback() Execution { return Execution{} }

// AuthAccount 给账号管理叶子使用普通生命周期。
func AuthAccount() Execution { return Normal() }

// AuthLogin 给浏览器登录使用普通生命周期。
func AuthLogin() Execution { return Normal() }

// AuthBundleImport 不检查自动更新，避免导入凭据后立即发起无关网络请求。
func AuthBundleImport() Execution {
	result := AuthAccount()
	result.AutomaticUpdate = false
	return result
}

// ConfigPath 显式列出与 Normal 相同的字段；config path 需要配置目录，但不执行数据命令。
func ConfigPath() Execution {
	return Execution{StartupHooks: true, EnsureConfig: true, AutomaticUpdate: true}
}

// UpdateCommand 不运行自动更新检查，避免 update 命令递归触发自己。
func UpdateCommand() Execution {
	return Execution{StartupHooks: true, EnsureConfig: true}
}

// PixivData 给 Pixiv 数据命令使用普通生命周期。
func PixivData() Execution { return Normal() }

// DownloadCommand 给下载命令使用普通生命周期。
func DownloadCommand() Execution { return Normal() }

// FanboxData 给 FANBOX 数据命令使用普通生命周期。
func FanboxData() Execution { return Normal() }

// FanboxAuth 不使用普通启动钩子：FANBOX 登录不需要 Pixiv 配置和默认文件的启动副作用。
func FanboxAuth() Execution {
	return Execution{StartupHooks: true, AutomaticUpdate: true}
}

// PixivMCP 启用启动钩子并确保配置，同时标记 MCP transport 以安装对应的管道策略。
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
