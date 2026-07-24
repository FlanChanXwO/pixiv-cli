# Explicit installation workflow

Use this workflow only when the user explicitly asks to install or repair the
`pixiv` binary. Installation changes files and may change the user PATH; state
the detected platform, architecture, destination, and PATH change before
running anything.

## Approved sources

- Repository: `https://github.com/FlanChanXwO/pixiv-cli`
- Unix installer:
  `https://github.com/FlanChanXwO/pixiv-cli/releases/latest/download/install.sh`
- Windows CMD installer:
  `https://raw.githubusercontent.com/FlanChanXwO/pixiv-cli/main/scripts/install.cmd`

Download and inspect the selected script before execution. Do not substitute a
mirror, custom base URL, package copied from chat, or an improvised installer.
The installer itself must select the latest stable official Release and report
`SHA-256 verified` before replacing a binary.

## Platform flow

1. Detect OS and architecture without reading Pixiv auth/config state.
2. Windows: use `install.cmd` through `cmd.exe`; never introduce a PowerShell
   dependency. Linux/macOS: use `install.sh` through `sh`.
3. Use a per-user destination. `--add-to-path` authorizes only adding that
   install directory to the current user's PATH; do not request administrator
   or root privileges.
4. If `curl`, `tar`, the checksum tool, or another declared prerequisite is
   missing, stop and ask before installing it. Do not manually substitute a
   download or extraction path: the official versioned installer alone may
   select its embedded public Release transport candidates after matching the
   direct GitHub checksum.
5. Never read authentication storage, import/export authentication, or request
   a Pixiv credential as part of installation.
6. Require installer success, then run `pixiv version`. Report the installed
   version, binary path, profile/registry PATH change, and any warning exactly.

If the user wants no PATH change, pass `--no-path`. A custom
`--install-dir DIR` is allowed, but on Unix the automatic `--add-to-path`
operation intentionally supports only the default `$HOME/.local/bin`; explain
the manual PATH step for another directory.
