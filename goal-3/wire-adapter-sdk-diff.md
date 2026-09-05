# Wire / adapter / SDK 差异表

只列缺口。

| Case | Wire | Response | Pagination | Adapter | SDK | 结论 |
| --- | --- | --- | --- | --- | --- | --- |
| novel-detail-v1 | confirmed | rejected | not_tested | not_tested | inconclusive | upstream_contract_rejected |
| novel-detail-v2 | confirmed | confirmed | confirmed | not_tested | not_tested | 生产层未覆盖 |
| novel-series-v1 | confirmed | rejected | not_tested | not_tested | inconclusive | required_field_missing |
| novel-series-v2 | confirmed | confirmed | inconclusive | not_tested | not_tested | second_page_not_observed |
| novel-content-app | confirmed | rejected | not_tested | not_tested | inconclusive | upstream_contract_rejected |
| novel-content-webview | confirmed | confirmed | confirmed | not_tested | not_tested | 生产层未覆盖 |
| novel-new | confirmed | confirmed | confirmed | not_tested | inconclusive | sdk_call_error |
| novel-ranking | confirmed | confirmed | confirmed | not_tested | not_tested | 生产层未覆盖 |
| user-novels | confirmed | confirmed | inconclusive | confirmed | confirmed | second_page_not_observed |
| user-novel-bookmarks-public | confirmed | confirmed | inconclusive | confirmed | confirmed | second_page_not_observed |
| user-novel-bookmarks-private | confirmed | confirmed | inconclusive | confirmed | confirmed | second_page_not_observed |
| novel-comments-v2 | confirmed | confirmed | inconclusive | confirmed | confirmed | second_page_not_observed |
| novel-comments-v3 | confirmed | confirmed | inconclusive | not_tested | not_tested | second_page_not_observed |
| search-illust-rating | confirmed | confirmed | confirmed | not_tested | not_tested | 生产层未覆盖 |
| illust-recommended | confirmed | confirmed | inconclusive | confirmed | confirmed | second_page_error |
| user-illusts-illust | confirmed | confirmed | inconclusive | confirmed | confirmed | second_page_not_observed |
| user-illusts-manga | confirmed | confirmed | inconclusive | confirmed | confirmed | second_page_not_observed |
| user-illust-bookmarks-public | confirmed | confirmed | inconclusive | confirmed | confirmed | second_page_not_observed |
| user-illust-bookmarks-private | confirmed | confirmed | inconclusive | confirmed | confirmed | second_page_not_observed |
| illust-comments-v3 | confirmed | confirmed | inconclusive | confirmed | confirmed | second_page_not_observed |
| stamps | confirmed | confirmed | confirmed | not_tested | not_tested | 生产层未覆盖 |
| illust-comment-text | confirmed | confirmed | confirmed | not_tested | not_tested | 生产层未覆盖 |
| illust-comment-reply | confirmed | confirmed | confirmed | not_tested | not_tested | 生产层未覆盖 |
| novel-comment-stamp | confirmed | confirmed | confirmed | not_tested | not_tested | 生产层未覆盖 |
| novel-comment-text | confirmed | confirmed | confirmed | not_tested | not_tested | 生产层未覆盖 |
| illust-comment-delete | confirmed | confirmed | confirmed | not_tested | not_tested | 生产层未覆盖 |
| novel-comment-delete | confirmed | confirmed | confirmed | not_tested | not_tested | 生产层未覆盖 |


## 原始计划的未覆盖项

当前 strict manifest 尚未覆盖：

- novel search `start_date/end_date`。
- novel bookmark tags/detail/add/delete。
- artwork bookmark tags/subtype。
- artwork recommended subtype。
- artwork latest 扩展 subtype。
- comments total 的非空语义。

这些项的 Wire、Adapter、SDK 均不能标为 confirmed。
