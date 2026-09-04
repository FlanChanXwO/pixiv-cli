# v1.0.2 — 2026-09-04

## 新增

- 新增 `sdk/pixiv.Client.SaveResourceURL`，并让下载 adapter 支持直接保存通过 allowlist 校验的 Pixiv CDN resource。该路径校验 HTTPS 与官方或显式允许的 host，复用 Pixiv referer 和 redirect 校验，禁用 cookies，并采用原子写入。([`1ac3e47`](https://github.com/FlanChanXwO/pixiv-cli/commit/1ac3e4750dde9221b94c43cf9ff19be234623772), [`b4e339c`](https://github.com/FlanChanXwO/pixiv-cli/commit/b4e339c15a276755f8acba2637e4b3aabc3d3ea8))

## 变更

- 在 CLI 与 MCP 下载中保留 structured batch result：后续 item 失败时仍保留已完成文件，逐项 failure 保留 typed cause，取消能够向上传递，账号池只会在尚未发布文件前重放调用。([`ee7be4a`](https://github.com/FlanChanXwO/pixiv-cli/commit/ee7be4afde62c448a370196f27771d7c3aaf3458), [`038208e`](https://github.com/FlanChanXwO/pixiv-cli/commit/038208e3a2bd7b6137f77d8cfbdd16ca54a259c1), [`9bd9701`](https://github.com/FlanChanXwO/pixiv-cli/commit/9bd9701d0bd5435ebd41061af716d3856f8da057), [`3c4d035`](https://github.com/FlanChanXwO/pixiv-cli/commit/3c4d03507ad1f7170ac2b348ef5b50a1ccd002d2))
- 将非阻断的 ugoira filename-template 问题作为带 safe fallback filename 的 warning 暴露，而不是让整个下载失败；CLI 将 warning 写入 stderr，MCP 在 structured result 中返回 warning。([`0e4980d`](https://github.com/FlanChanXwO/pixiv-cli/commit/0e4980d980bb17e78fbd7caeeb4f33f615adb2c8))

## 修复

- 按 API response 顺序生成多页作品的 page index 并构造互不冲突的 resource reference，避免下载时页面 collision 并保持页选择语义。([`5d25741`](https://github.com/FlanChanXwO/pixiv-cli/commit/5d2574190f2424630635c0b715d5d3cc2e3c13bd))
- 将静态图片 MIME type 统一映射为稳定的 `.jpg`、`.png`、`.gif`、`.webp` 扩展名，并显式拒绝不支持的图片类型。([`33c915d`](https://github.com/FlanChanXwO/pixiv-cli/commit/33c915da8e5c778ae7b9a1bb0c533f7c9ef09871))

## 文档

- 同步 CLI、MCP、SDK、README 与产品 skill 文档，覆盖 direct-resource download、quality 与 page selection、structured output、warning 以及 delivery 语义。([`a023e14`](https://github.com/FlanChanXwO/pixiv-cli/commit/a023e1483a94771e6fac312ff4f9db69e626d560), [`4eb436c`](https://github.com/FlanChanXwO/pixiv-cli/commit/4eb436cd02085931e50620efa14457874d2732d1))

## 维护

- 完成 direct-resource flow audit，并扩展 page ordering、CDN policy、MIME mapping、partial result、cancellation 以及 CLI/MCP projection 的下载与 report adapter 覆盖。([`b5fff06`](https://github.com/FlanChanXwO/pixiv-cli/commit/b5fff0603e2cfe1a2495f6d30c6bf207ab2759ac), [`1abbf46`](https://github.com/FlanChanXwO/pixiv-cli/commit/1abbf467e6c35d14918b54e9d64216b8282712e6))

**完整变更**：[v1.0.1...v1.0.2](https://github.com/FlanChanXwO/pixiv-cli/compare/v1.0.1...v1.0.2)
