# Inspect config unset syntax

## Command
```shell
cd /private/tmp/pixiv-cli-e2e-shell-20260821 && ./bin/pixiv config unset --help
```

## Output
```text
Remove one config value from config.toml. KEY must be one of: account_pool_enabled, account_pool_strategy, directory_template, download_path, filename_template, https_proxy, log_format, log_level, request_interval

Usage:
  pixiv config unset KEY [flags]

Flags:
  -h, --help   help for unset
```

Exit code: 0

Verdict: PASS
