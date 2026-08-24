# Inspect auth export syntax

## Command
```shell
cd /private/tmp/pixiv-cli-e2e-shell-20260821 && ./bin/pixiv auth export --help
```

## Output
```text
Export stored authentication

Usage:
  pixiv auth export [UID] [flags]

Flags:
      --all             export all stored accounts; cannot be combined with UID
      --force           replace an existing output file; requires --output
  -h, --help            help for export
      --output string   write a versioned authentication bundle to PATH
```

Exit code: 0

Verdict: PASS
