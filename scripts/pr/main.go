// pr 命令实现仓库的受信 PR 门禁：校验 PR 元数据、解析 /test 触发与验证命令，
// 并在各平台执行声明过的命令、汇总结果。
//
// 包的组成：[Stage]、[Command] 与 [Commands] 表示声明过的验证命令，[Whitelist] 持有
// 允许执行的命令和路径规则。[LoadWhitelist]、[Whitelist.Validate]、[ParseLine] 与
// [NormalizeAndHash] 构成解析与校验入口。子命令分发在 main，触发与有效命令解析在
// resolve.go，执行与结果渲染在 verify.go。
package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fatal(fmt.Errorf("expected a PR subcommand: validate, trigger, resolve, init, execute, or render"))
	}
	switch os.Args[1] {
	case "validate":
		runMeta(os.Args[2:])
	case "trigger":
		runTrigger(os.Args[2:])
	case "resolve":
		runResolve(os.Args[2:])
	case "init", "execute", "render":
		runVerify(os.Args[1:])
	default:
		fatal(fmt.Errorf("unknown PR subcommand %q", os.Args[1]))
	}
}
