# v1.0.2 — 2026-09-10

## 新增

- SDK 的 `SaveResourceURL` 与 CLI/MCP 下载命令可以直接保存 Pixiv CDN 资源，并保留所选质量和页码。`pixiv detail` 可以接收搜索或反向搜图得到的作品、小说、用户记录，并自动推断类型。（[#76](https://github.com/FlanChanXwO/pixiv-cli/pull/76), [#79](https://github.com/FlanChanXwO/pixiv-cli/pull/79)）
- Release 完成后会把经过验证的容器镜像发布到 Docker Hub：`docker.io/flanchanxwo/pixiv-cli`。（[#80](https://github.com/FlanChanXwO/pixiv-cli/pull/80)）

## 变更

- 批量下载在后续条目失败时保留已完成文件，逐项报告错误，并在取消时立即停止。无法套用的动图文件名模板会给出警告并使用后备名称，而不是让整个下载失败。（[#79](https://github.com/FlanChanXwO/pixiv-cli/pull/79)）

## 修复

- 多页作品的页号按 `meta_pages` 顺序生成，即使上游响应省略 `page_index`；`--pages` 和整部作品下载不再把不同页映射到同一张图片。（[#79](https://github.com/FlanChanXwO/pixiv-cli/pull/79), [#81](https://github.com/FlanChanXwO/pixiv-cli/pull/81)）
- 静态图片使用稳定的 `.jpg`、`.png`、`.gif` 或 `.webp` 扩展名，不支持的图片类型会明确报错。（[#79](https://github.com/FlanChanXwO/pixiv-cli/pull/79)）

## 安全

- 直接保存 CDN 资源时会重新校验每次跳转，不转发你的 cookie，签名 URL 不会出现在结果或诊断中；写入失败不会留下部分文件。（[#79](https://github.com/FlanChanXwO/pixiv-cli/pull/79)）

**完整变更**：[v1.0.1...v1.0.2](https://github.com/FlanChanXwO/pixiv-cli/compare/v1.0.1...v1.0.2)
