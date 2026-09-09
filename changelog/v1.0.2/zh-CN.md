# v1.0.2 — 2026-09-09

## 新增

- 通过 `sdk/pixiv.Client.SaveResourceURL` 新增 Pixiv CDN direct-resource 保存，包含 HTTPS/host/redirect 校验、Pixiv referer、cookie 隔离与原子 destination 写入；CLI/MCP 下载 adapter 使用同一边界，并保持所选 quality 与 page 语义。([#79](https://github.com/FlanChanXwO/pixiv-cli/pull/79))
- `pixiv detail` 现在可以消费普通 search 或 reverse-search pipeline 产生的 canonical artwork、novel、user record，推断兼容的 entity 类型，并把详情结果重新投影为 canonical record 输出，同时保留原有 raw ID/URL 文本路径。([#76](https://github.com/FlanChanXwO/pixiv-cli/pull/76))
- 在成功的 Release workflow 之后，把同一组经过验证的原生 `linux/amd64` 与 `linux/arm64` release 镜像发布到 Docker Hub：`docker.io/flanchanxwo/pixiv-cli`。post-Release publisher 复用 immutable verified artifact，并支持使用原始 release tag 与 run ID 显式恢复，而不是重新构建镜像。([#80](https://github.com/FlanChanXwO/pixiv-cli/pull/80))

## 变更

- 在 CLI 与 MCP 下载中保留 structured batch result：后续 item 失败时仍保留已完成文件，逐项 failure 保留 typed cause，取消能够向上传递，账号池只会在尚未发布文件前重放调用。非阻断的 ugoira filename-template 问题现在返回带 safe fallback filename 的 warning，而不是让整个下载失败。([#79](https://github.com/FlanChanXwO/pixiv-cli/pull/79))

## 修复

- 按 `meta_pages` 数组顺序统一生成 Pixiv 多页作品的 page index。最终的 detail-path 回归修复覆盖真实响应省略 `page_index` 的情况，使每页获得不同的 resource reference，`--pages` 指定页下载或整部作品下载不再把不同输出文件错误解析成同一张图片。([#79](https://github.com/FlanChanXwO/pixiv-cli/pull/79)、[#81](https://github.com/FlanChanXwO/pixiv-cli/pull/81))
- 将静态图片 MIME type 统一映射为稳定的 `.jpg`、`.png`、`.gif`、`.webp` 扩展名，并显式拒绝不支持的图片类型。([#79](https://github.com/FlanChanXwO/pixiv-cli/pull/79))

## 安全

- 将 CDN 直接保存限制在 SDK resource-policy 边界内：每次 redirect 都重新校验，不转发 caller cookie，带签名的 source URL 不进入公开下载结果或诊断，失败写入也不会发布部分 destination file。([#79](https://github.com/FlanChanXwO/pixiv-cli/pull/79))
- Docker Hub credential 只存在于受保护的 `release` Environment，并仅通过 `docker login --password-stdin` 注入；发布前会验证 immutable tag、公开 Release、source run、镜像 architecture 与 OCI provenance label。([#80](https://github.com/FlanChanXwO/pixiv-cli/pull/80))

## 文档

- 同步 CLI、MCP、SDK、README、维护者与产品 skill 文档，覆盖 direct-resource download、quality 与闭区间 page selection、structured result 与 warning、canonical detail pipeline，以及 Docker Hub 发布与恢复契约。([#76](https://github.com/FlanChanXwO/pixiv-cli/pull/76)、[#79](https://github.com/FlanChanXwO/pixiv-cli/pull/79)、[#80](https://github.com/FlanChanXwO/pixiv-cli/pull/80))

## 维护

- 扩展 page ordering、CDN policy、MIME mapping、partial result、cancellation、CLI/MCP projection、多页 resource identity 与 verified container provenance 的回归和 release-contract 覆盖。([#79](https://github.com/FlanChanXwO/pixiv-cli/pull/79)、[#80](https://github.com/FlanChanXwO/pixiv-cli/pull/80)、[#81](https://github.com/FlanChanXwO/pixiv-cli/pull/81))

**完整变更**：[v1.0.1...v1.0.2](https://github.com/FlanChanXwO/pixiv-cli/compare/v1.0.1...v1.0.2)
