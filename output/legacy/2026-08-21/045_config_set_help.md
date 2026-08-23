# Inspect config set syntax

## Command
```shell
cd /private/tmp/pixiv-cli-e2e-shell-20260821 && ./bin/pixiv config set --help
```

## Output
```text
Set one config value in config.toml. KEY must be one of: account_pool_enabled, account_pool_strategy, directory_template, download_path, filename_template, https_proxy, log_format, log_level, request_interval

Usage:
  pixiv config set KEY [VALUE] [flags]

Flags:
  -h, --help   help for set
```

Exit code: 0

Verdict: PASS
