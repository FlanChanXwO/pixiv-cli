// Package requirements 描述每个 CLI 命令的执行生命周期需求。
// 它只暴露生命周期事实，不提供数据库或 client getter，避免命令注册演变为 service locator。
package requirements

import (
	"sync"

	"github.com/spf13/cobra"
)

// Resources 标识命令 RunE 前必须就绪的私有 run graph 节点。
// composition root 将这些声明转换为具体资源。
type Resources struct {
	ConfigSnapshot bool
	Database       bool
	PixivAccount   bool
	PixivLogin     bool
	PixivSDK       bool
	Download       bool
	Fanbox         bool
	Update         bool
}

// Execution 是命令 owner 声明的启动契约。命令包构造 Cobra command 时绑定它，
// root 仅执行契约。
type Execution struct {
	StartupHooks    bool
	EnsureConfig    bool
	Diagnostics     bool
	AutomaticUpdate bool
	MCP             bool
	Resources       Resources
}

var commandRequirements sync.Map // map[*cobra.Command]Execution

// Bind 为一个 command constructor 注册静态需求。
func Bind(command *cobra.Command, requirement Execution) {
	if command == nil {
		return
	}
	commandRequirements.Store(command, requirement)
}

// Override 在 owner 专属输入分类后替换命令需求。auth bundle import 确认输入为
// 离线 bundle 后、root startup hook 运行前使用它。
func Override(command *cobra.Command, requirement Execution) {
	Bind(command, requirement)
}

// For 返回最近的 owner 声明。group command 可为后代声明共同需求，特殊 child
// 则可覆盖它，而无需 root 解析 command path。
func For(command *cobra.Command) Execution {
	for current := command; current != nil; current = current.Parent() {
		if requirement, ok := commandRequirements.Load(current); ok {
			return requirement.(Execution)
		}
	}
	return Execution{StartupHooks: true, Diagnostics: true}
}

// Clear 在一次嵌入式 CLI 运行结束后释放 command 指针。
func Clear(root *cobra.Command) {
	if root == nil {
		return
	}
	commandRequirements.Delete(root)
	for _, command := range root.Commands() {
		Clear(command)
	}
}

// Normal 返回普通成功 CLI leaf 的共同契约。
func Normal(resources Resources) Execution {
	return Execution{
		StartupHooks:    true,
		EnsureConfig:    true,
		Diagnostics:     true,
		AutomaticUpdate: true,
		Resources:       resources,
	}
}

// AuthExport 保护唯一允许输出 raw token 的 stdout 路由：跳过所有通用启动副作用，
// 但保留本地账号数据库读取。
func AuthExport() Execution {
	return Execution{Resources: Resources{Database: true, PixivAccount: true}}
}

// HiddenCallback 使协议 callback 与 handler installer 路由不创建配置、不执行通用
// startup hook/diagnostics，也不访问数据库。
func HiddenCallback() Execution { return Execution{} }

// AuthAccount 覆盖普通 Pixiv 账号管理命令。
func AuthAccount() Execution {
	return Normal(Resources{Database: true, PixivAccount: true})
}

// AuthLogin 覆盖交互式登录流程：它需要本地账号 store 和 login adapter，但不预先
// 打开 SDK 网络操作。
func AuthLogin() Execution {
	return Normal(Resources{Database: true, PixivLogin: true})
}

// AuthBundleImport 只在 auth owner 分类出有效 bundle 后选用。它保持离线，并跳过
// automatic-update hook。
func AuthBundleImport() Execution {
	result := AuthAccount()
	result.AutomaticUpdate = false
	return result
}

// ConfigPath 保留既有行为：输出路径前先创建最小默认配置，随后仍可执行普通
// startup 与 automatic-update 流程。
func ConfigPath() Execution {
	return Execution{StartupHooks: true, EnsureConfig: true, Diagnostics: true, AutomaticUpdate: true}
}

// Version 不依赖 config 或业务资源。
func Version() Execution { return Execution{StartupHooks: true, Diagnostics: true} }

// UpdateCommand 只需要 config snapshot 与 update 构造。
func UpdateCommand() Execution {
	return Execution{
		StartupHooks: true,
		EnsureConfig: true,
		Diagnostics:  true,
		Resources: Resources{
			ConfigSnapshot: true,
			Update:         true,
		},
	}
}

// PixivData 覆盖普通 Pixiv 数据命令。
func PixivData() Execution {
	return Normal(Resources{ConfigSnapshot: true, Database: true, PixivSDK: true})
}

// DownloadCommand 在普通 Pixiv 数据依赖上增加 media factory。
func DownloadCommand() Execution {
	return Normal(Resources{ConfigSnapshot: true, Database: true, PixivSDK: true, Download: true})
}

// FanboxData 覆盖普通 FANBOX 数据命令。
func FanboxData() Execution {
	return Normal(Resources{ConfigSnapshot: true, Database: true, Fanbox: true})
}

// FanboxAuth 不得仅因管理 session 而创建默认配置文件。
func FanboxAuth() Execution {
	return Execution{
		StartupHooks:    true,
		Diagnostics:     true,
		AutomaticUpdate: true,
		Resources:       Resources{Database: true, Fanbox: true},
	}
}

// PixivMCP 声明隔离的 stdio runtime，并显式禁用 automatic update，确保 stdout
// 只承载 JSON-RPC。
func PixivMCP() Execution {
	return Execution{
		StartupHooks: true,
		EnsureConfig: true,
		Diagnostics:  true,
		MCP:          true,
		Resources: Resources{
			ConfigSnapshot: true,
			Database:       true,
			PixivSDK:       true,
			Download:       true,
		},
	}
}

// FanboxMCP 是 FANBOX 对应的隔离 stdio runtime 声明。
func FanboxMCP() Execution {
	return Execution{
		StartupHooks: true,
		EnsureConfig: true,
		Diagnostics:  true,
		MCP:          true,
		Resources: Resources{
			ConfigSnapshot: true,
			Database:       true,
			Fanbox:         true,
		},
	}
}
