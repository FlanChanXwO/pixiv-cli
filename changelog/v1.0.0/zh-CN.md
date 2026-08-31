# v1.0.0 — 2026-08-31

## 破坏性变更

- 完成 v1 SDK 与 CLI 重写，将 Pixiv/FANBOX public SDK、CLI、MCP、认证、存储和资源契约统一到 v1 架构。 ([#59](https://github.com/FlanChanXwO/pixiv-cli/pull/59))

## 新增

- 为 CLI 与 Pixiv MCP 新增反向搜图，支持本地文件/URL source 识别、SauceNAO 与 ascii2d provider、稳定结果 envelope 以及明确的 provider partial 语义。 ([#62](https://github.com/FlanChanXwO/pixiv-cli/pull/62))
- 新增一等 Docker 容器发布支持，包含固定 runtime、provenance、非 root 状态处理和精确版本发布校验。 ([#63](https://github.com/FlanChanXwO/pixiv-cli/pull/63))

## 变更

- 按已验证的响应契约和 public SDK 类型统一 Pixiv timeline 与 MyPixiv 操作。 ([#60](https://github.com/FlanChanXwO/pixiv-cli/pull/60))

## 修复

- 恢复经审计的 v0.10.0 recovery 路径发布校验，避免有效的发布准备被拒绝。 ([#49](https://github.com/FlanChanXwO/pixiv-cli/pull/49))
- 处理 ClawHub pending security scan，避免已接收的发布被错误报告为失败。 ([#50](https://github.com/FlanChanXwO/pixiv-cli/pull/50))
- 修复 Pixiv 认证服务初始化错误、已验证的当前用户与端点契约、FANBOX 代理冲突校验，以及本地账号数据库的向前兼容迁移。 ([#67](https://github.com/FlanChanXwO/pixiv-cli/pull/67))
- 修复 Windows 发布门禁在文件 URI、ACL 权限、平台路径和可执行文件命名上的兼容性问题。 ([#68](https://github.com/FlanChanXwO/pixiv-cli/pull/68))

## 文档

- 简化 pull request template，使审查记录包含仓库流程要求的变更、验证和自查证据。 ([#61](https://github.com/FlanChanXwO/pixiv-cli/pull/61))
- 在双语 v1.0.0 release notes 中记录经审计的 Windows 兼容性修复。 ([#69](https://github.com/FlanChanXwO/pixiv-cli/pull/69))
- 完成 v1.0.0 release notes 的审计来源覆盖。 ([#70](https://github.com/FlanChanXwO/pixiv-cli/pull/70))

## 维护

- 移除仓库中的 generated goal 与 output artifact。 ([#64](https://github.com/FlanChanXwO/pixiv-cli/pull/64))
- 移除未使用的仓库 artifact，保持提交的 source tree 聚焦。 ([#65](https://github.com/FlanChanXwO/pixiv-cli/pull/65))
- 简化过大的 Go 测试文件，同时保留 installer、FANBOX 和其他行为覆盖。 ([#66](https://github.com/FlanChanXwO/pixiv-cli/pull/66))

**完整变更**：[v0.10.0...v1.0.0](https://github.com/FlanChanXwO/pixiv-cli/compare/v0.10.0...v1.0.0)
