# v1.0.0 严格 unknown-option 解析

状态：设计已确认，RC-2 已实施并通过聚焦回归。确认日期：2026-08-04；实施记录：2026-08-08。

## 结论

现有 Cobra/pflag 已拒绝大部分 unknown flag，但错误文本与 exit classification 尚未形成稳定契约。
v1.0.0 RC follow-up 在 root command boundary 统一规范所有未注册 option，不把规则复制到各子命令：

```text
error: unknown option '--api-url'
```

## 解析与错误契约

- `-` 或 `--` 开头、位于 end-of-options marker `--` 之前的 token 由 pflag 严格解析；未在目标命令、
  parent 或 root 注册的 option 立即失败，不能作为位置参数、透传输入或 ignored compatibility flag。
- option 写在位置参数前后都服从相同规则；这保留项目既有的 interspersed flag 行为。
- `--api-url=value` 只引用 option spelling，输出 `error: unknown option '--api-url'`，不回显 value。
- 未知短 option 输出 `error: unknown option '-x'`；组合 shorthand 报告解析到的第一个未知 short name，
  不把整段 token 或后续字符当作错误内容。
- stdout 必须为空；错误包装为现有 `usageError`，process exit code 固定为 `2`。
- 参数解析失败发生在 config initialization、账号选择、网络、文件副作用、MCP runtime 与 command
  debug lifecycle 之前。即使 argv 同时含有效 `--debug`，也只有上述单一错误行。
- `--` 保留标准 end-of-options 语义；其后的 `--api-url` 是位置值，再由目标命令既有 Args contract
  判断。需要搜索或下载一个以 `-` 开头的合法值时，调用者必须使用该 marker。
- unknown command 继续使用既有 `unknown command`；位置参数缺失/过多继续使用对应 `usage: ...`。
  known option 缺值、非法 value 或互斥 option 的既有具体错误不在本次统一改名范围。

root-level flag error normalization 只识别 pflag/Cobra 的 unknown-option 分类并生成稳定 safe message，
不能依赖易变的完整上游英文错误字符串。它不增加每个 command 的 allowlist，也不读取 raw argv value
作为错误正文。

## 与 debug 的关系

unknown option 在 diagnostic scope 创建前失败，因此 `--debug` 不为参数解析错误额外输出
`[Pixiv CLI]`、`[FANBOX CLI]` 或任何 lifecycle event。debug 的完整输出和 writer failure 契约见
[显式 debug 诊断](debug-diagnostics.md)。

## 测试与验收

聚焦测试至少覆盖：

- root、普通 Pixiv command、FANBOX command、`pixiv mcp` 与 `pixiv fanbox mcp`；
- unknown long option、short option、组合 shorthand 与 `--name=value`；
- option 位于位置参数之前和之后；
- `--` 后以 `-` 开头的合法 literal；
- stderr exact message、stdout empty 与 exit code `2`；
- 解析失败没有触发 config、SDK、network、文件副作用、MCP runtime 或 debug presenter。

## 非目标

- 不增加 fuzzy suggestion、自动拼写修正或 hidden pass-through list。
- 不把 `--` 后的合法位置值重新解释成 option。
- 不统一重写所有 usage/value/mutual-exclusion 错误。
- 不为兼容旧 wrapper 静默忽略已删除 option；旧 option 只能由明确 migration tombstone 处理。
