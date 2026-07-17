# ADR 0007: 提交平台 Staticlib 以维持受支持的源码构建

## Status

Accepted；完整六目标由 pinned native runner 验证并受控回填，run `29567721284` 已完成本次重建。

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
- release workflow 必须把生成 committed staticlib 的 Rust toolchain 作为 target provenance 固定：
  `x86_64-apple-darwin` 与 `x86_64-pc-windows-msvc` 使用 `1.96.0`，其余四个 release target 使用
  `1.96.1`。test 与 production job 必须从同一受审计 matrix 选择版本，禁止使用可移动的 `stable`
  或 runner 默认值；production 仍只 checkout immutable tag，toolchain pin 不构成源码 overlay。
- native-evidence workflow 必须使用同一目标映射：matrix 显式携带版本，job 通过
  `RUSTUP_TOOLCHAIN` 绑定，并以 `rustup toolchain install ... --no-self-update` 安装版本和目标；
  release 与 native-evidence verifier 共用唯一 policy 映射，禁止三份独立事实源漂移。
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
- run `29567721284`（head `a93378631654f7a19b5e6052f68bdb3650438b03`）已按上述
  `1.96.0`/`1.96.1` 映射完整重建六目标，并通过 policy、精确源码 ref、locked/offline build、
  GIF/APNG/cgo smoke、archive/record 与 artifact upload。下载六份证据后的本地 fail-closed
  consolidation 继续核对 version、commit、source digest、目标集合与逐库 hash，随后将六库及 manifest
  成套回填。临时 review ref 仅用于合并前精确 checkout 该受审计 commit；回填后 workflow 已恢复只接受
  `refs/heads/main`。旧 run `29559729696` 的 runner 默认 Rust `1.97.0` 产物不得作为 recovery byte rebuild
  的合规证据，也不得只改 manifest 或文字声明来冒充重建。
- 上述 per-target pin 是当前六库及 v0.3.0 immutable-tag recovery 的实际 provenance。后续 Rust
  升级必须从同一受审计 source 成套重建、链接并 smoke 验证六目标，同时更新 staticlib、manifest、
  native evidence 与 release matrix；不能单独漂移某个 target 或用新编译器覆盖既有 tag 的 library。
- Cargo `--locked --offline` 现在经 crate 内 source replacement 使用完整 vendor 闭包；空 Cargo cache
  的 metadata/build/test 与六 target 许可证检查证明 native Rust 输入不依赖开发机或 runner registry cache。
