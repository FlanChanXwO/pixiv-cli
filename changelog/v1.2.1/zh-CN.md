# v1.2.1 — 2026-10-22

## 变更

- 文档重组为聚焦的独立页面，使用表格和图示，覆盖安装、认证、MCP 图库和维护者指南；快速开始命令已修正，README 增加贡献者展示链接。（[#154](https://github.com/FlanChanXwO/pixiv-cli/pull/154), [#156](https://github.com/FlanChanXwO/pixiv-cli/pull/156)）
- 根目录 `LICENSE` 恢复为纯 MIT 文本，GitHub 可以正确识别许可证。第三方声明仍可通过 `pixiv --licenses` 查看。（[#155](https://github.com/FlanChanXwO/pixiv-cli/pull/155)）

## 修复

- 修复 Windows 账号存储文件监听器的竞态（fsnotify 升级到 v1.10.1），长时间运行的 CLI 与 MCP 会话不再出现并发访问错误。（[#153](https://github.com/FlanChanXwO/pixiv-cli/pull/153)）

## 安全

- 源码构建与发布二进制所需的 Go 工具链提升到 1.27.3。Go 1.27.3 在 `crypto/x509` 中拒绝包含特定不可信中间证书的证书链（CVE-2026-111943），并包含 Go 1.27.2 的修复。从源码构建时需要升级工具链。（[#158](https://github.com/FlanChanXwO/pixiv-cli/pull/158)）

**完整变更**：[v1.2.0...v1.2.1](https://github.com/FlanChanXwO/pixiv-cli/compare/v1.2.0...v1.2.1)

<!-- internal:
  [#157](https://github.com/FlanChanXwO/pixiv-cli/pull/157)
-->
