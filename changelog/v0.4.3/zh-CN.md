# v0.4.3 — 2026-07-19

## 修复

- 修复 Linux Homebrew Release validation 使用 `/var/tmp` 时可能触发 `EINVAL`、导致 staging formula 无法安装的问题；该验证步骤现仅在 Linux 使用 runner 私有临时目录，macOS 与公开 formula 路径保持不变。
