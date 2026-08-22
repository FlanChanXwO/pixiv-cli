# Inspect config command syntax

## Command
```shell
cd /private/tmp/pixiv-cli-e2e-shell-20260821 && ./bin/pixiv config --help
```

## Output
```text
Manage global Pixiv CLI settings

Usage:
  pixiv config [flags]
  pixiv config [command]

Available Commands:
  get         Print one effective config value
  path        Print the config.toml path
  set         Set one config value in config.toml
  unset       Remove one config value from config.toml

Flags:
  -h, --help   help for config

Use "pixiv config [command] --help" for more information about a command.
```

Exit code: 0

Verdict: PASS
