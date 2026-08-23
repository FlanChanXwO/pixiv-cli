# Compare exposed Pixiv read-only commands with completed and pending e2e tasks

## Command
```shell
cd /private/tmp/pixiv-cli-e2e-shell-20260821 && printf "%s\n" "completed_read_only_groups: search detail ranking trending-tags recommended timeline user mypixiv auth" "pending_read_only_groups: novel-search series comments bookmark-read-only user-novels user-related user-followers user-blocked" "pending_disk_group: download" "pending_error-network-group: invalid-args proxy anonymous-fallback" "write_operations_excluded: bookmark-add bookmark-remove follow-add follow-remove" && echo "root_commands_from_help:" && sed -n "/Available Commands:/,/Flags:/p" /private/tmp/pixiv-cli-e2e-shell-20260821/root-help-snapshot.txt 2>/dev/null || echo "snapshot_not_saved; source report=output/009_root_help.md" && echo "planned_tasks:" && rg -n "Task (9|10|11)|novel|series|comment|bookmark|download|fallback" /Users/flanchan/Developer/Projects/GithubProjects/pixiv-cli/goal-1/tasks.md
```

## Output
```text
completed_read_only_groups: search detail ranking trending-tags recommended timeline user mypixiv auth
pending_read_only_groups: novel-search series comments bookmark-read-only user-novels user-related user-followers user-blocked
pending_disk_group: download
pending_error-network-group: invalid-args proxy anonymous-fallback
write_operations_excluded: bookmark-add bookmark-remove follow-add follow-remove
root_commands_from_help:
snapshot_not_saved; source report=output/009_root_help.md
planned_tasks:
53:- 实际完成：在隔离账号和显式代理下执行 recommended artwork/novel/user/all NDJSON，timeline following/latest page 1/page 2，MyPixiv users 和 works illust/novel。following 与 MyPixiv 空数组均为成功响应；latest page 2 成功返回不同数组。运行时发现 `mypixiv works` 帮助写 `artwork`，省略 USER_ID 实际只接受 `illust|novel`，保留失败并按实际值重试成功。
54:- 验证证据：`output/101_recommended_artwork_json.md`、`output/102_recommended_novel_json.md`、`output/103_recommended_user_json.md`、`output/104_recommended_all_ndjson.md`、`output/105_timeline_following_json.md`、`output/106_timeline_latest_json.md`、`output/107_timeline_latest_page2_json.md`、`output/108_mypixiv_users_json.md`、`output/109_mypixiv_works_json.md`、`output/110_mypixiv_works_illust_json.md`、`output/111_mypixiv_works_novel_json.md`、`output/112_task6_json_shape_validation.md`、`output/113_task6_report_audit.md`、`output/114_timeline_latest_page_distinct.md`。latest 使用 `--page 2` 的真实 shell 成功证据与修复分支中 `max_illust_id` continuation 单测共同证明该路径。
56:- 下一步：进入 Task 7，覆盖用户搜索、详情、artworks、bookmarks、following 及当前用户路径。
60:- [x] 覆盖 user search/detail/artworks/bookmarks/following。
62:- 实际完成：以用户搜索得到的 UID `7621567` 执行显式 user detail、artworks、public bookmarks、following；省略 USER_ID 执行当前账号 artworks/bookmarks/following；另以账号 UID `25649510` 执行用户 detail，验证当前账户 profile 返回。所有命令使用隔离 HOME、显式代理和正数 limit。
63:- 验证证据：`output/115_user_search_miku_json.md`、`output/116_user_detail_json.md`、`output/117_user_artworks_explicit_json.md`、`output/118_user_bookmarks_explicit_json.md`、`output/119_user_following_explicit_json.md`、`output/120_current_user_artworks_json.md`、`output/121_current_user_bookmarks_json.md`、`output/122_current_user_following_json.md`、`output/123_current_user_detail_id_json.md`、`output/124_task7_json_shape_validation.md`、`output/125_task7_report_audit.md`。
75:## Task 9：小说与 NDJSON/filter 管道 e2e
77:- [ ] 覆盖 novel search。
84:## Task 10：下载与 MIME/扩展名 e2e
94:## Task 11：错误路径、代理与匿名 fallback e2e
97:- [ ] 在适用范围验证匿名 web fallback；直连失败时显式测试本次命令代理。
```

Exit code: 0

Verdict: PASS
