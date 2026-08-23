# Inspect update command syntax

## Command
```shell
cd /private/tmp/pixiv-cli-e2e-shell-20260821 && ./bin/pixiv update --help
```

## Output
```text
Check for or install updates

Usage:
  pixiv update [flags]

Examples:
pixiv update --check

Flags:
      --check          check for an update without installing it
  -h, --help           help for update
      --json           print update check status as JSON (requires --check)
      --prerelease     include prerelease updates
      --proxy string   HTTP(S) proxy URL for this update command
```

Exit code: 0

Verdict: PASS
