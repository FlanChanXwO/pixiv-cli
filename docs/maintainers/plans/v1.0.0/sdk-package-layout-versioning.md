# v1.0.0 公开 SDK package 布局与版本管理

## 状态

已采纳，目标版本 v1.0.0。

本计划决策取代 [ADR 0009](../../adr/0009-public-pixiv-sdk-and-caller-adapter.md) 中顶层公开 package `/pixiv` 的
路径决策；不改变其 public SDK 与调用方 adapter 分离的原则。

## 背景

v1 同时提供 Pixiv App API 与 Pixiv FANBOX。把两个产品全部放进 package `pixiv` 虽然不会让 receiver
method 发生命名冲突，但会把 constructors、credentials、options、request 和领域模型堆进同一导出
namespace。分别新增顶层 `/pixiv`、`/fanbox` 和 `/sdk` 又会让仓库顶层公共入口过多。

候选结构还包括 `/sdk/pixiv/v1` 与 `/sdk/fanbox/v1`。Go 允许普通 package 目录使用 `v1`，但 minor/
patch 版本仍由所属 module 选择；目录 `/v1` 不能让同一个 build 同时选择 v1.0.2 和 v1.1.0，也会与
未来 module major suffix 形成两套版本含义。

## 决策

- 根 module 保持 `github.com/FlanChanXwO/pixiv-cli`，SDK、CLI 与 FANBOX 使用同一 release version。
- v1 公开 package 固定为：

  ```text
  github.com/FlanChanXwO/pixiv-cli/sdk
  github.com/FlanChanXwO/pixiv-cli/sdk/pixiv
  github.com/FlanChanXwO/pixiv-cli/sdk/fanbox
  ```

- `sdk` 只提供协议无关的分页、cursor、错误和资源契约；两个产品 package 只依赖 `sdk`，彼此不依赖。
- package path 不带 `/v1`。v1.0.2、v1.1.0 等版本通过根 module 的完整、不可变 SemVer tag 选择。
- 不为两个产品创建独立 `go.mod` submodule；除非未来有真实、持续的独立发布节奏需求，否则不承担
  多 module tag、依赖与 release orchestration。
- v1.0.0 删除 v0 顶层 `/pixiv`，不提供 compatibility package。迁移指南提供完整 import/symbol
  mapping；v1.0.0 之后不得移动或删除上述三个 package。
- 所有公开 package 使用英文 GoDoc；internal adapter 继续位于根 module 的 `internal/`。

## 版本获取语义

旧 tag 保存旧目录快照，因此 main 后续删除 `/pixiv` 不影响其他项目使用：

```bash
go get github.com/FlanChanXwO/pixiv-cli/pixiv@v0.10.0
go get github.com/FlanChanXwO/pixiv-cli/sdk/pixiv@v1.0.0
go get github.com/FlanChanXwO/pixiv-cli/sdk/pixiv@v1.1.0
```

不同消费者可以固定不同 tag，本机 module cache 也可保存多个版本；同一个 build 对同一 module path
只选择一个版本，所以不能同时编译 v0 `/pixiv` 与 v1 `/sdk/pixiv`。v1 的 minor/patch release 必须
向后兼容，不能以“旧 tag 仍可获取”为理由在 v1.1.0 删除 v1.0.2 的 API。

## 后果

- 仓库顶层只有一个明确的公共 SDK 入口 `/sdk`，产品 namespace 与共享基础层边界清楚。
- 调用方获得自然的 `pixiv.Open`、`fanbox.Open` 和 `sdk.Page/Error/Resource`，无需 `OpenFANBOX`
  一类前缀。
- CLI 与两个 SDK 产品同版本发布，测试矩阵和 release notes 不需要组合多个 module version。
- v0 调用方必须在迁移到 v1 时一次修改 import path；同一进程无法用根 module 的两个版本做渐进迁移。
- 如果未来真的发布 module v2，应遵循 Go module major-version 规则整体设计，而不是提前在 v1 package
  path 中加入冗余版本目录。
