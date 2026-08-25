# Inspect auth command syntax

## Command
```shell
cd /private/tmp/pixiv-cli-e2e-shell-20260821 && ./bin/pixiv auth --help
```

## Output
```text
Manage local Pixiv authentication

Usage:
  pixiv auth [flags]
  pixiv auth [command]

Available Commands:
  check            Validate an account token
  export           Export stored authentication
  import           Import or replace an account
  list             List accounts
  login            Login with the Pixiv browser OAuth flow
  pool             Manage account pool scheduling
  refresh          Refresh account credentials and membership status
  remove           Remove an account
  use              Set the default account

Flags:
  -h, --help   help for auth

Use "pixiv auth [command] --help" for more information about a command.
```

Exit code: 0

Verdict: PASS
