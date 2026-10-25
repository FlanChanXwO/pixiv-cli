# v0.1.0 — 2026-07-13

## 破坏性变更

- 本地账号改用 Pixiv UID 标识，`pixiv auth` 相关命令用数字 Pixiv ID 指代账号。浏览器登录也可以在远程机器上完成。（[`3bf5fc4`](https://github.com/FlanChanXwO/pixiv-cli/commit/3bf5fc4), [`53ad400`](https://github.com/FlanChanXwO/pixiv-cli/commit/53ad400)）

## 新增

- `pixiv update` 可安装签名的发布版本、识别当前安装来源、在有新版本时提示更新；`pixiv --version` 现在可以显示构建信息。（[`1101530`](https://github.com/FlanChanXwO/pixiv-cli/commit/1101530), [`3176147`](https://github.com/FlanChanXwO/pixiv-cli/commit/3176147), [`75872cd`](https://github.com/FlanChanXwO/pixiv-cli/commit/75872cd), [`a2330b4`](https://github.com/FlanChanXwO/pixiv-cli/commit/a2330b4), [`a279985`](https://github.com/FlanChanXwO/pixiv-cli/commit/a279985), [`c4e7737`](https://github.com/FlanChanXwO/pixiv-cli/commit/c4e7737)）
- 动图作品下载时可用内置 GIF/APNG 编码，不再需要外部编码器。（[`235a017`](https://github.com/FlanChanXwO/pixiv-cli/commit/235a017), [`e5bc6a9`](https://github.com/FlanChanXwO/pixiv-cli/commit/e5bc6a9)）
- 首次发布 `pixiv` CLI 与 MCP server：账号命令、API 读取、下载和安装使用文档。（[`51b6894`](https://github.com/FlanChanXwO/pixiv-cli/commit/51b6894), [`53ad400`](https://github.com/FlanChanXwO/pixiv-cli/commit/53ad400)）

## 修复

- 下载和更新在全部六个发布平台上可用，包括用 7-Zip 打包的 Windows 压缩包和 `install.cmd` 安装脚本。（[`0a36d3d`](https://github.com/FlanChanXwO/pixiv-cli/commit/0a36d3d), [`4a315a0`](https://github.com/FlanChanXwO/pixiv-cli/commit/4a315a0), [`a092dd8`](https://github.com/FlanChanXwO/pixiv-cli/commit/a092dd8), [`ab03362`](https://github.com/FlanChanXwO/pixiv-cli/commit/ab03362), [`c295394`](https://github.com/FlanChanXwO/pixiv-cli/commit/c295394), [`c5eef2c`](https://github.com/FlanChanXwO/pixiv-cli/commit/c5eef2c), [`d486442`](https://github.com/FlanChanXwO/pixiv-cli/commit/d486442), [`e5a8ff3`](https://github.com/FlanChanXwO/pixiv-cli/commit/e5a8ff3), [`fc2d7d7`](https://github.com/FlanChanXwO/pixiv-cli/commit/fc2d7d7)）
- 更新不会再留下损坏的可执行文件：替换正在运行的二进制、符号链接、取消安装和临时文件清理都已安全处理。（[`6f47ead`](https://github.com/FlanChanXwO/pixiv-cli/commit/6f47ead), [`ab03362`](https://github.com/FlanChanXwO/pixiv-cli/commit/ab03362), [`beb97e1`](https://github.com/FlanChanXwO/pixiv-cli/commit/beb97e1), [`d11653e`](https://github.com/FlanChanXwO/pixiv-cli/commit/d11653e)）
- 更新下载会拒绝逃逸目标目录的压缩包路径，也不再信任未校验的 GitHub 资源地址。（[`0643c40`](https://github.com/FlanChanXwO/pixiv-cli/commit/0643c40), [`79568f9`](https://github.com/FlanChanXwO/pixiv-cli/commit/79568f9), [`7c04d19`](https://github.com/FlanChanXwO/pixiv-cli/commit/7c04d19), [`808f132`](https://github.com/FlanChanXwO/pixiv-cli/commit/808f132), [`e1ee586`](https://github.com/FlanChanXwO/pixiv-cli/commit/e1ee586)）
- 动图下载遇到缺少原生编码器的构建会明确报错，不再静默生成损坏的二进制；发布构建也会拒绝不匹配的原生编码器。（[`0a36d3d`](https://github.com/FlanChanXwO/pixiv-cli/commit/0a36d3d), [`42a5515`](https://github.com/FlanChanXwO/pixiv-cli/commit/42a5515), [`981cf14`](https://github.com/FlanChanXwO/pixiv-cli/commit/981cf14)）

**完整变更**：[v0.1.0 commits](https://github.com/FlanChanXwO/pixiv-cli/commits/v0.1.0)

<!-- internal:
  [`02cc1bb`](https://github.com/FlanChanXwO/pixiv-cli/commit/02cc1bb)
  [`08b98c8`](https://github.com/FlanChanXwO/pixiv-cli/commit/08b98c8)
  [`0cbd004`](https://github.com/FlanChanXwO/pixiv-cli/commit/0cbd004)
  [`0d6772a`](https://github.com/FlanChanXwO/pixiv-cli/commit/0d6772a)
  [`112f15d`](https://github.com/FlanChanXwO/pixiv-cli/commit/112f15d)
  [`18a6431`](https://github.com/FlanChanXwO/pixiv-cli/commit/18a6431)
  [`19d5ca1`](https://github.com/FlanChanXwO/pixiv-cli/commit/19d5ca1)
  [`1e3005b`](https://github.com/FlanChanXwO/pixiv-cli/commit/1e3005b)
  [`1ee15ad`](https://github.com/FlanChanXwO/pixiv-cli/commit/1ee15ad)
  [`2490f03`](https://github.com/FlanChanXwO/pixiv-cli/commit/2490f03)
  [`2706c15`](https://github.com/FlanChanXwO/pixiv-cli/commit/2706c15)
  [`272c4aa`](https://github.com/FlanChanXwO/pixiv-cli/commit/272c4aa)
  [`2cff4b6`](https://github.com/FlanChanXwO/pixiv-cli/commit/2cff4b6)
  [`4002c10`](https://github.com/FlanChanXwO/pixiv-cli/commit/4002c10)
  [`49fba31`](https://github.com/FlanChanXwO/pixiv-cli/commit/49fba31)
  [`4e93760`](https://github.com/FlanChanXwO/pixiv-cli/commit/4e93760)
  [`4f811b8`](https://github.com/FlanChanXwO/pixiv-cli/commit/4f811b8)
  [`4fc43e4`](https://github.com/FlanChanXwO/pixiv-cli/commit/4fc43e4)
  [`4ff6f12`](https://github.com/FlanChanXwO/pixiv-cli/commit/4ff6f12)
  [`537159b`](https://github.com/FlanChanXwO/pixiv-cli/commit/537159b)
  [`5572733`](https://github.com/FlanChanXwO/pixiv-cli/commit/5572733)
  [`557b473`](https://github.com/FlanChanXwO/pixiv-cli/commit/557b473)
  [`557beb0`](https://github.com/FlanChanXwO/pixiv-cli/commit/557beb0)
  [`597d1bc`](https://github.com/FlanChanXwO/pixiv-cli/commit/597d1bc)
  [`5e39c06`](https://github.com/FlanChanXwO/pixiv-cli/commit/5e39c06)
  [`69f45cf`](https://github.com/FlanChanXwO/pixiv-cli/commit/69f45cf)
  [`72cb4fa`](https://github.com/FlanChanXwO/pixiv-cli/commit/72cb4fa)
  [`7420040`](https://github.com/FlanChanXwO/pixiv-cli/commit/7420040)
  [`7c16333`](https://github.com/FlanChanXwO/pixiv-cli/commit/7c16333)
  [`86697e9`](https://github.com/FlanChanXwO/pixiv-cli/commit/86697e9)
  [`87442be`](https://github.com/FlanChanXwO/pixiv-cli/commit/87442be)
  [`8caecda`](https://github.com/FlanChanXwO/pixiv-cli/commit/8caecda)
  [`8f7169a`](https://github.com/FlanChanXwO/pixiv-cli/commit/8f7169a)
  [`91154f0`](https://github.com/FlanChanXwO/pixiv-cli/commit/91154f0)
  [`9968e0e`](https://github.com/FlanChanXwO/pixiv-cli/commit/9968e0e)
  [`9c9071e`](https://github.com/FlanChanXwO/pixiv-cli/commit/9c9071e)
  [`a1c6b83`](https://github.com/FlanChanXwO/pixiv-cli/commit/a1c6b83)
  [`a468ea6`](https://github.com/FlanChanXwO/pixiv-cli/commit/a468ea6)
  [`a5592c2`](https://github.com/FlanChanXwO/pixiv-cli/commit/a5592c2)
  [`abdde72`](https://github.com/FlanChanXwO/pixiv-cli/commit/abdde72)
  [`aed109d`](https://github.com/FlanChanXwO/pixiv-cli/commit/aed109d)
  [`b127db5`](https://github.com/FlanChanXwO/pixiv-cli/commit/b127db5)
  [`b1367be`](https://github.com/FlanChanXwO/pixiv-cli/commit/b1367be)
  [`b1dac08`](https://github.com/FlanChanXwO/pixiv-cli/commit/b1dac08)
  [`b864329`](https://github.com/FlanChanXwO/pixiv-cli/commit/b864329)
  [`baf0f95`](https://github.com/FlanChanXwO/pixiv-cli/commit/baf0f95)
  [`c228b86`](https://github.com/FlanChanXwO/pixiv-cli/commit/c228b86)
  [`c732bd2`](https://github.com/FlanChanXwO/pixiv-cli/commit/c732bd2)
  [`c7ed6e5`](https://github.com/FlanChanXwO/pixiv-cli/commit/c7ed6e5)
  [`c83acb4`](https://github.com/FlanChanXwO/pixiv-cli/commit/c83acb4)
  [`d4036cf`](https://github.com/FlanChanXwO/pixiv-cli/commit/d4036cf)
  [`e7df9f2`](https://github.com/FlanChanXwO/pixiv-cli/commit/e7df9f2)
  [`ecccff3`](https://github.com/FlanChanXwO/pixiv-cli/commit/ecccff3)
  [`f7926ae`](https://github.com/FlanChanXwO/pixiv-cli/commit/f7926ae)
  [`f7a9f46`](https://github.com/FlanChanXwO/pixiv-cli/commit/f7a9f46)
-->
