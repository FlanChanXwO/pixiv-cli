# v0.5.0 — 2026-07-22

## 破坏性变更

- `pixiv search` 与 MCP 的搜索/列表工具只接受新的筛选字段，旧的搜索与账号兼容参数已移除；`pixiv download` 的参数只在该命令下可用。（[`27c151b`](https://github.com/FlanChanXwO/pixiv-cli/commit/27c151b), [`41430fe`](https://github.com/FlanChanXwO/pixiv-cli/commit/41430fe), [`7249b1d`](https://github.com/FlanChanXwO/pixiv-cli/commit/7249b1d), [`8502851`](https://github.com/FlanChanXwO/pixiv-cli/commit/8502851)）
- MCP 下载改为交付本地文件，并显式支持 `pages` 与 `quality` 选项，不再返回内联内容。（[`8502851`](https://github.com/FlanChanXwO/pixiv-cli/commit/8502851)）

## 新增

- CLI 与 MCP 下载命令支持选择页码和静态图片质量，作品记录新增稳定的作品 URL。（[`bc00211`](https://github.com/FlanChanXwO/pixiv-cli/commit/bc00211), [`ecfc1ee`](https://github.com/FlanChanXwO/pixiv-cli/commit/ecfc1ee)）
- CLI 可使用 App API 的全部排行模式。（[`bc00211`](https://github.com/FlanChanXwO/pixiv-cli/commit/bc00211)）
- `debug` 会在用户状态目录下写入脱敏的每日操作日志。（[`907fe1b`](https://github.com/FlanChanXwO/pixiv-cli/commit/907fe1b)）

## 修复

- 已认证的 R18 作品读取恢复；搜索筛选会跳过空的上游批次，登录会显示最终页面，命令结束时日志写入器会关闭。（[`1030531`](https://github.com/FlanChanXwO/pixiv-cli/commit/1030531), [`1e01b74`](https://github.com/FlanChanXwO/pixiv-cli/commit/1e01b74), [`274ccac`](https://github.com/FlanChanXwO/pixiv-cli/commit/274ccac), [`71734d0`](https://github.com/FlanChanXwO/pixiv-cli/commit/71734d0), [`79c14ac`](https://github.com/FlanChanXwO/pixiv-cli/commit/79c14ac), [`8a172da`](https://github.com/FlanChanXwO/pixiv-cli/commit/8a172da)）
- AI 生成作品筛选优先使用上游当前的 `illust_ai_type` 字段，筛选结果与 Pixiv 显示一致。（[`1729806`](https://github.com/FlanChanXwO/pixiv-cli/commit/1729806), [`8505651`](https://github.com/FlanChanXwO/pixiv-cli/commit/8505651)）
- CLI 提示已统一为英文，搜索参数的说明更清楚。（[`76d8e7b`](https://github.com/FlanChanXwO/pixiv-cli/commit/76d8e7b), [`b6c0ac8`](https://github.com/FlanChanXwO/pixiv-cli/commit/b6c0ac8), [`d13a4a1`](https://github.com/FlanChanXwO/pixiv-cli/commit/d13a4a1)）

**完整变更**：[v0.4.5...v0.5.0](https://github.com/FlanChanXwO/pixiv-cli/compare/v0.4.5...v0.5.0)

<!-- internal:
  [`1ffad53`](https://github.com/FlanChanXwO/pixiv-cli/commit/1ffad53)
  [`02fe9f5`](https://github.com/FlanChanXwO/pixiv-cli/commit/02fe9f5)
  [`058aaad`](https://github.com/FlanChanXwO/pixiv-cli/commit/058aaad)
  [`0acc0d7`](https://github.com/FlanChanXwO/pixiv-cli/commit/0acc0d7)
  [`0e23694`](https://github.com/FlanChanXwO/pixiv-cli/commit/0e23694)
  [`26cf51b`](https://github.com/FlanChanXwO/pixiv-cli/commit/26cf51b)
  [`3053df6`](https://github.com/FlanChanXwO/pixiv-cli/commit/3053df6)
  [`a58f261`](https://github.com/FlanChanXwO/pixiv-cli/commit/a58f261)
  [`afbb293`](https://github.com/FlanChanXwO/pixiv-cli/commit/afbb293)
  [`cd8d278`](https://github.com/FlanChanXwO/pixiv-cli/commit/cd8d278)
  [`f026316`](https://github.com/FlanChanXwO/pixiv-cli/commit/f026316)
  [`fba19f0`](https://github.com/FlanChanXwO/pixiv-cli/commit/fba19f0)
-->
