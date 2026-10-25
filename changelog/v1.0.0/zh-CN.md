# v1.0.0 — 2026-08-31

## 破坏性变更

- v1 重写改变了公开的 Pixiv/FANBOX SDK、CLI、MCP、认证、存储和资源契约。SDK 使用方需要按 v1 API 重新核对导入和类型用法。（[#59](https://github.com/FlanChanXwO/pixiv-cli/pull/59)）

## 新增

- `pixiv search` 与 Pixiv MCP 的 `reverse_search` 工具新增反向搜图：支持本地文件和 URL、SauceNAO 与 ascii2d provider、稳定的结果结构，并明确报告部分 provider 失败。（[#62](https://github.com/FlanChanXwO/pixiv-cli/pull/62)）
- 发布官方 `linux/amd64` 与 `linux/arm64` 容器镜像，使用固定运行时、非 root 状态处理和版本校验。（[#63](https://github.com/FlanChanXwO/pixiv-cli/pull/63)）

## 变更

- Pixiv 动态（timeline）与 MyPixiv 读取已按验证过的响应契约和公开 SDK 类型统一。（[#60](https://github.com/FlanChanXwO/pixiv-cli/pull/60)）

## 修复

- `pixiv auth` 初始化与当前用户读取恢复正常，FANBOX 会拒绝冲突的代理设置，已有账号数据库会原地迁移。（[#67](https://github.com/FlanChanXwO/pixiv-cli/pull/67)）
- Windows 构建与安装器可正确处理文件 URI、ACL 权限、平台路径和可执行文件名称。（[#68](https://github.com/FlanChanXwO/pixiv-cli/pull/68)）
- 发布构建中的动图编码使用与 tag 源码匹配的库，重建可以得到相同的字节。（[#72](https://github.com/FlanChanXwO/pixiv-cli/pull/72)）
- 容器镜像报告的版本与 release tag 一致。（[#74](https://github.com/FlanChanXwO/pixiv-cli/pull/74)）

## 安全

- ClawHub 仍在安全审核时会报告为待审核，不再显示为发布失败；v0.10.0 发布失败后也可以恢复。（[#49](https://github.com/FlanChanXwO/pixiv-cli/pull/49), [#50](https://github.com/FlanChanXwO/pixiv-cli/pull/50)）

**完整变更**：[v0.10.0...v1.0.0](https://github.com/FlanChanXwO/pixiv-cli/compare/v0.10.0...v1.0.0)

<!-- internal:
  [#61](https://github.com/FlanChanXwO/pixiv-cli/pull/61)
  [#64](https://github.com/FlanChanXwO/pixiv-cli/pull/64)
  [#65](https://github.com/FlanChanXwO/pixiv-cli/pull/65)
  [#66](https://github.com/FlanChanXwO/pixiv-cli/pull/66)
  [#69](https://github.com/FlanChanXwO/pixiv-cli/pull/69)
  [#70](https://github.com/FlanChanXwO/pixiv-cli/pull/70)
  [#71](https://github.com/FlanChanXwO/pixiv-cli/pull/71)
  [#73](https://github.com/FlanChanXwO/pixiv-cli/pull/73)
-->
