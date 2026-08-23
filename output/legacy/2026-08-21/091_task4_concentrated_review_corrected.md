# Concentrated review of Tasks 1-3 evidence

## Command
```shell
cd /private/tmp/pixiv-cli-e2e-shell-20260821 && zsh ./audit-task4.zsh
```

## Output
```text
report_statuses:
006_build_pixiv.md | Exit code: 0 | Verdict: PASS
007_binary_metadata.md | Exit code: 0 | Verdict: PASS
008_version_json.md | Exit code: 1 | Verdict: FAIL
009_root_help.md | Exit code: 0 | Verdict: PASS
010_auth_help.md | Exit code: 0 | Verdict: PASS
011_bookmark_help.md | Exit code: 0 | Verdict: PASS
012_comment_help.md | Exit code: 0 | Verdict: PASS
013_config_help.md | Exit code: 0 | Verdict: PASS
014_detail_help.md | Exit code: 0 | Verdict: PASS
015_download_help.md | Exit code: 0 | Verdict: PASS
016_follow_help.md | Exit code: 0 | Verdict: PASS
017_mcp_help.md | Exit code: 0 | Verdict: PASS
018_mypixiv_help.md | Exit code: 0 | Verdict: PASS
019_novel_help.md | Exit code: 0 | Verdict: PASS
020_ranking_help.md | Exit code: 0 | Verdict: PASS
021_recommended_help.md | Exit code: 0 | Verdict: PASS
022_search_help.md | Exit code: 0 | Verdict: PASS
023_series_help.md | Exit code: 0 | Verdict: PASS
024_timeline_help.md | Exit code: 0 | Verdict: PASS
025_update_help.md | Exit code: 0 | Verdict: PASS
026_user_help.md | Exit code: 0 | Verdict: PASS
027_root_help_batch.md | Exit code: 0 | Verdict: PASS
028_nested_command_inventory.md | Exit code: 0 | Verdict: PASS
029_auth_check_help.md | Exit code: 0 | Verdict: PASS
030_auth_export_help.md | Exit code: 0 | Verdict: PASS
031_auth_import_help.md | Exit code: 0 | Verdict: PASS
032_auth_list_help.md | Exit code: 0 | Verdict: PASS
033_auth_login_help.md | Exit code: 0 | Verdict: PASS
034_auth_pool_help.md | Exit code: 0 | Verdict: PASS
035_auth_refresh_help.md | Exit code: 0 | Verdict: PASS
036_auth_remove_help.md | Exit code: 0 | Verdict: PASS
037_auth_use_help.md | Exit code: 0 | Verdict: PASS
038_bookmark_add_help.md | Exit code: 0 | Verdict: PASS
039_bookmark_detail_help.md | Exit code: 0 | Verdict: PASS
040_bookmark_list_help.md | Exit code: 0 | Verdict: PASS
041_bookmark_remove_help.md | Exit code: 0 | Verdict: PASS
042_bookmark_tags_help.md | Exit code: 0 | Verdict: PASS
043_config_get_help.md | Exit code: 0 | Verdict: PASS
044_config_path_help.md | Exit code: 0 | Verdict: PASS
045_config_set_help.md | Exit code: 0 | Verdict: PASS
046_config_unset_help.md | Exit code: 0 | Verdict: PASS
047_follow_add_help.md | Exit code: 0 | Verdict: PASS
048_follow_remove_help.md | Exit code: 0 | Verdict: PASS
049_mypixiv_users_help.md | Exit code: 0 | Verdict: PASS
050_mypixiv_works_help.md | Exit code: 0 | Verdict: PASS
051_novel_search_help.md | Exit code: 0 | Verdict: PASS
052_timeline_following_help.md | Exit code: 0 | Verdict: PASS
053_timeline_latest_help.md | Exit code: 0 | Verdict: PASS
054_user_artworks_help.md | Exit code: 0 | Verdict: PASS
055_user_blocked_help.md | Exit code: 0 | Verdict: PASS
056_user_bookmarks_help.md | Exit code: 0 | Verdict: PASS
057_user_detail_help.md | Exit code: 0 | Verdict: PASS
058_user_follow_help.md | Exit code: 0 | Verdict: PASS
059_user_followers_help.md | Exit code: 0 | Verdict: PASS
060_user_following_help.md | Exit code: 0 | Verdict: PASS
061_user_novels_help.md | Exit code: 0 | Verdict: PASS
062_user_related_help.md | Exit code: 0 | Verdict: PASS
063_user_search_help.md | Exit code: 0 | Verdict: PASS
064_nested_help_batch.md | Exit code: 0 | Verdict: PASS
065_deep_command_inventory.md | Exit code: 0 | Verdict: PASS
066_auth_pool_disable_help.md | Exit code: 0 | Verdict: PASS
067_auth_pool_enable_help.md | Exit code: 0 | Verdict: PASS
068_auth_pool_status_help.md | Exit code: 0 | Verdict: PASS
069_user_follow_add_help.md | Exit code: 0 | Verdict: PASS
070_user_follow_remove_help.md | Exit code: 0 | Verdict: PASS
071_deep_help_batch.md | Exit code: 0 | Verdict: PASS
072_key_read_command_syntax.md | Exit code: 0 | Verdict: PASS
073_task2_report_audit.md | Exit code: 1 | Verdict: FAIL
074_root_version_correct_syntax.md | Exit code: 0 | Verdict: PASS
075_task2_report_audit.md | Exit code: 0 | Verdict: PASS
076_auth_list_json.md | Exit code: 1 | Verdict: FAIL
077_config_path_current_env.md | Exit code: 0 | Verdict: PASS
078_local_state_inventory.md | Exit code: 127 | Verdict: FAIL
079_local_state_inventory_corrected.md | Exit code: 1 | Verdict: FAIL
080_installed_pixiv_auth_list.md | Exit code: 0 | Verdict: PASS
081_auth_export_bundle.md | Exit code: 0 | Verdict: PASS
082_auth_import_isolated.md | Exit code: 2 | Verdict: FAIL
083_auth_import_bundle_stdin.md | Exit code: 0 | Verdict: PASS
084_auth_list_isolated.md | Exit code: 0 | Verdict: PASS
085_auth_check_isolated.md | Exit code: 1 | Verdict: FAIL
086_auth_check_proxy.md | Exit code: 0 | Verdict: PASS
087_auth_log_secret_audit.md | Exit code: 1 | Verdict: FAIL
089_auth_log_secret_audit.md | Exit code: 0 | Verdict: PASS
output_non_markdown:
none
secret_value_patterns:
none
command_workdir_check:
output_used_as_workdir
```

Exit code: 1

Verdict: FAIL
