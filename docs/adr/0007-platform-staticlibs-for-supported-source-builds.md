# ADR 0007: 提交平台 Staticlib 以维持受支持的源码构建

## Status

Accepted；完整六目标交付仍待 native runner 验证。

## Context

ugoira GIF/APNG encoder 由 Rust 实现，并通过 cgo 接入 Go CLI。让每个终端用户在 `go build` 或
`go install` 时即时编译 Rust，会把 Rust toolchain、target、C linker 与网络/registry 状态变成
隐式安装前提，也会让同一 Go tag 的 native 行为不可审计。运行时调用 `ffmpeg` 则会重新引入外部
二进制依赖，无法满足内置 encoder 的发布要求。

项目必须同时支持 darwin、linux、windows 的 amd64/arm64。每个 target 的 Rust archive 都是不同
二进制输入，不能由 host archive 复制、改名或用 Git LFS pointer 代替。要将这些二进制和 Rust
source 的关系保留给发布审计，需要可重算的 source digest、每文件 SHA-256 和稳定的 target mapping。

## Decision

- 为六个 GOOS/GOARCH target 使用明确的 Rust target staticlib 与 cgo selector；受支持源码构建要求
  Go `1.26.3`、`CGO_ENABLED=1`、目标 C linker 和对应 staticlib。
- 将经 native runner 生成、链接和 smoke 验证的 staticlib 以普通 Git blob 提交，而非 Git LFS；
  `staticlib/manifest.json` 必须同时记录 six target、path、SHA-256 与 Rust source digest。
- `scripts/build-staticlibs.sh` 仅在同一份 source 成功取得全部六个真实库后写 manifest。单 target
  生成会使旧 manifest 失效，不能被误用为跨平台发布证明。
- Rust `target/` 始终是忽略的机器产物；已验证 staticlib 与 manifest 是可审计发布输入，不能忽略。
- production ugoira 路径只使用 Rust encoder；`ffmpeg` 仅保留给显式启用的开发质量对照，不作为运行
  时 fallback。

## Consequences

- 发行 archive 和 future `go install` 可以链接已验证的 target library，而不是在用户机器上临时
  解决 Rust build 环境。
- 仓库会包含较大的 native binary blob，必须在提交/发布前审查 source digest、manifest、hash 与
  native runner 证据。
- 没有 cgo、target C linker、目标 staticlib 或完整 manifest 时，构建必须清晰失败；不能退回 stub、
  `ffmpeg` 或“部分可用”的 binary。
- 当前仅保存 Darwin/arm64 staticlib，且没有完整 manifest。因此这项决策尚未让 source build、
  `go install` 或 Release 成为跨平台可发布路径；Task 13/33 仍需取得五个真实库、同源 manifest 与
  每平台 GIF/APNG/cgo smoke 证据。
- 当前 Cargo `--locked --offline` 仍可能依赖开发机 registry cache。Task 31 在 fresh cache 下完成
  complete vendor/source replacement 前，不能声称 native 输入可脱离本地缓存复现。
