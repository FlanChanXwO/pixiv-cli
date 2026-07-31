# CLI、MCP 与下载体验修复设计

## 目标

本轮以稳定、可见的用户行为为中心：信息流统一为 `timeline`，绘图工具筛选
使用版本内置目录，下载能够在交互终端显示真实字节进度，MCP 的读取结果具备
一致的过滤和错误契约。浏览器登录与新的设置向导不在范围内。

## 边界

CLI 与 MCP 继续只经 `internal/application` 调用顶层 `pixiv` SDK。下载资源的
HEAD 探测是内部 transport 能力，复用既有资源 policy、redirect 校验、referer 与
cookie 禁用规则；公开 `OpenResource` 仍是 GET API。

`DownloadOptions.Progress` 是纯观察、可并发调用的 SDK hook。取消由调用方的
context 负责，SDK 不从回调读取控制值。总大小未知不会阻止下载：CLI 不显示条，
SDK 仍报告资源的已传输字节。

## 兼容性与安全

`feed`、`search-options`、动态 `SearchIllustOptions` 以及四个旧 MCP tool 是有意
删除的破坏性接口。静态绘图工具目录固定为 Pixiv 菜单的 101 个原始去重值，数据
保存在 `pixiv/drawing-tools.json` 并随 Go 二进制嵌入；校验不接受别名。近似建议仅
用于唯一的单编辑纠错，含混前缀只链接文档。

账号池仍只重放尚未提交结果的安全读取；429 日志只保存前后 UID、操作和安全的
Retry-After 元数据。匿名列表补全失败直接暴露调用失败，不伪造部分成功。
