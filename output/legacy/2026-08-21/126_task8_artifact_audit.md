# Audit all Pixiv e2e report artifacts and sensitive-output constraints

## Command
```shell
cd /private/tmp/pixiv-cli-e2e-shell-20260821 && zsh ./audit-task8.zsh
```

## Output
```text
report_format_invalid:
output_non_markdown:
none
output_workdir_hits:
none
secret_value_hits:
none
temp_workdir_report_count:
114
report_count:
124
known_failures:
008_version_json.md
073_task2_report_audit.md
076_auth_list_json.md
078_local_state_inventory.md
079_local_state_inventory_corrected.md
082_auth_import_isolated.md
085_auth_check_isolated.md
087_auth_log_secret_audit.md
090_task4_concentrated_review.md
091_task4_concentrated_review_corrected.md
096_search_options_legacy_probe.md
098_search_json_shape.md
109_mypixiv_works_json.md
```

Exit code: 0

Verdict: PASS
