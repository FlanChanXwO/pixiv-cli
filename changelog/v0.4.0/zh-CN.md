# v0.4.0 — 2026-07-19

## 破坏性变更

- 以安全的导入/导出流程替代旧版 `auth add` 与 `auth token`，并让已认证搜索遵循 App API 契约。([#14](https://github.com/FlanChanXwO/pixiv-cli/pull/14))

## 新增

- 新增 macOS/Linux 与 Windows 独立安装器、版本化离线认证 bundle、覆盖 SDK/CLI/MCP 的完整搜索筛选、多语言公开文档和 `pixiv-cli` 产品 Skill。([#14](https://github.com/FlanChanXwO/pixiv-cli/pull/14))

## 修复

- 固定原生 Rust provenance，并收窄 v0.3 恢复 overlay，使不可变发布 tag 从经审核的源码集合重新构建。([#8](https://github.com/FlanChanXwO/pixiv-cli/pull/8), [#9](https://github.com/FlanChanXwO/pixiv-cli/pull/9))

## 文档

- 整合 SDK 文档，并完成本次发布准备使用的 v0.3 最终审计记录。([#10](https://github.com/FlanChanXwO/pixiv-cli/pull/10), [#11](https://github.com/FlanChanXwO/pixiv-cli/pull/11), [#13](https://github.com/FlanChanXwO/pixiv-cli/pull/13))

## 维护

- 确立 Linux `GLIBC_2.35` 发布基线，并处理发布审查发现。([#12](https://github.com/FlanChanXwO/pixiv-cli/pull/12), [#14](https://github.com/FlanChanXwO/pixiv-cli/pull/14))

**完整变更**：[v0.3.0...v0.4.0](https://github.com/FlanChanXwO/pixiv-cli/compare/v0.3.0...v0.4.0)
