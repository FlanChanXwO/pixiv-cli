# Inspect auth pool syntax

## Command
```shell
cd /private/tmp/pixiv-cli-e2e-shell-20260821 && ./bin/pixiv auth pool --help
```

## Output
```text
Manage account pool scheduling

Usage:
  pixiv auth pool [flags]
  pixiv auth pool [command]

Available Commands:
  disable     Disable accounts in the pool
  enable      Enable accounts in the pool
  status      Show account pool scheduling status

Flags:
  -h, --help   help for pool

Use "pixiv auth pool [command] --help" for more information about a command.
```

Exit code: 0

Verdict: PASS
