# v0.4.5 — 2026-07-20

## 修复

- Linux Homebrew hosted staging verification 现在会在以不可变 digest 固定的短生命周期 `homebrew/brew` 容器中运行真实的本地 staging-tap `brew install`。这避免了 hosted Linuxbrew `Resource` staging cleanup `EINVAL`，同时保留只读 formula mount、无 secret、本地专用 tap 和精确的安装版本校验。固定的 Homebrew 4.6 镜像不带 `brew trust` 或独立 `python3`，因此 trust 仍只在原生 macOS 路径执行，容器则在安装后使用内置 Ruby JSON parser。Linux 容器 tap 在本地创建、只接收只读挂载内容，并通过 `--rm` 丢弃。macOS 与终端用户的 Homebrew 安装不变。GitHub prepublish workflow 已使用发布的 v0.4.4 资产通过四种 Homebrew 平台/架构组合的验证。
