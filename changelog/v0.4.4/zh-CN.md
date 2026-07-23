# v0.4.4 — 2026-07-19

## 修复

- Linux Homebrew release validation 将 buildpath 保持在 Homebrew prefix 内，避免跨文件系统触发 `FileUtils EINVAL`。
