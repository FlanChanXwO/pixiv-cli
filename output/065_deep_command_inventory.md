# Inventory deeper command levels

## Command
```shell
cd /Users/flanchan/Developer/Projects/GithubProjects/pixiv-cli && for report in output/0{29..63}_*.md; do if rg -q "Available Commands:" "$report"; then echo "== $report =="; sed -n "/Available Commands:/,/Flags:/p" "$report"; fi; done
```

## Output
```text
== output/034_auth_pool_help.md ==
Available Commands:
  disable     Disable accounts in the pool
  enable      Enable accounts in the pool
  status      Show account pool scheduling status

Flags:
== output/058_user_follow_help.md ==
Available Commands:
  add         Follow a user
  remove      Unfollow a user

Flags:
```

Exit code: 0

Verdict: PASS
