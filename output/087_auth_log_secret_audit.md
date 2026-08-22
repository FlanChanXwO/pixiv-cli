# Audit authentication reports for exposed credential values

## Command
```shell
cd /Users/flanchan/Developer/Projects/GithubProjects/pixiv-cli && printf "token_like_values:\n"; rg -n -i "(refresh_token|access_token|PHPSESSID)[[:space:]]*([=:])[[:space:]]*[^[:space:]`\"'{}]+" output/076_auth_list_json.md output/080_installed_pixiv_auth_list.md output/081_auth_export_bundle.md output/082_auth_import_isolated.md output/083_auth_import_bundle_stdin.md output/084_auth_list_isolated.md output/085_auth_check_isolated.md output/086_auth_check_proxy.md || true; printf "bundle_mode:\n"; stat -f "%Sp %z" /private/tmp/pixiv-cli-e2e-shell-20260821/credentials/pixiv-auth.bundle; printf "isolated_db_mode:\n"; stat -f "%Sp %z" /private/tmp/pixiv-cli-e2e-shell-20260821/home/.pixiv-cli/pixiv-cli.db
```

## Output
```text
zsh:1: unmatched "
```

Exit code: 1

Verdict: FAIL
