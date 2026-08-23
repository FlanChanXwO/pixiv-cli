# Audit Task 9 Pixiv command surface evidence

## Command
```shell
cd /private/tmp/pixiv-cli-e2e-shell-20260821 && zsh ./audit-task9.zsh
```

## Output
```text
expected_failure_missing=/Users/flanchan/Developer/Projects/GithubProjects/pixiv-cli/output/145_ndjson_search_pipeline.md
novel_search=pass
bookmark_list_tags=pass
user_novels_related_followers=pass
ndjson_jq_pipeline=pass
comments_series_bookmark_detail_blocked_filter_and_blocked_users=fail_recorded
```

Exit code: 1

Verdict: FAIL
