# Pixiv App API strict upstream evidence

| Case | Method | Path | HTTP | Page 1 | Page 2 | Continuation | Wire | Response | Pagination | Adapter | SDK | Verdict | Failure |
| --- | --- | --- | ---: | ---: | ---: | --- | --- | --- | --- | --- | --- | --- | --- |
| novel-detail-v1 | GET | /v1/novel/detail | 404 | 0 | 0 | none | confirmed | rejected | not_tested | not_tested | inconclusive | rejected | upstream_contract_rejected |
| novel-detail-v2 | GET | /v2/novel/detail | 200 | 0 | 0 | none | confirmed | confirmed | confirmed | not_tested | not_tested | not_tested |  |
| novel-series-v1 | GET | /v1/novel/series | 200 | 29 | 0 | none | confirmed | rejected | not_tested | not_tested | inconclusive | rejected | required_field_missing |
| novel-series-v2 | GET | /v2/novel/series | 200 | 29 | 0 | none | confirmed | confirmed | inconclusive | not_tested | not_tested | inconclusive | second_page_not_observed |
| novel-content-app | GET | /v1/novel/content | 404 | 0 | 0 | none | confirmed | rejected | not_tested | not_tested | inconclusive | rejected | upstream_contract_rejected |
| novel-content-webview | GET | /webview/v2/novel | 200 | 1 | 0 | none | confirmed | confirmed | confirmed | not_tested | not_tested | not_tested |  |
| novel-new | GET | /v1/novel/new | 200 | 30 | 30 | max_novel_id | confirmed | confirmed | confirmed | not_tested | inconclusive | inconclusive | sdk_call_error |
| novel-follow | GET | /v1/novel/follow | 200 | 30 | 30 | offset | confirmed | confirmed | confirmed | confirmed | confirmed | confirmed |  |
| novel-recommended | GET | /v1/novel/recommended | 200 | 33 | 33 | offset | confirmed | confirmed | confirmed | confirmed | confirmed | confirmed |  |
| novel-ranking | GET | /v1/novel/ranking | 200 | 30 | 30 | offset | confirmed | confirmed | confirmed | not_tested | not_tested | not_tested |  |
| user-novels | GET | /v1/user/novels | 200 | 0 | 0 | none | confirmed | confirmed | inconclusive | confirmed | confirmed | inconclusive | second_page_not_observed |
| user-novel-bookmarks-public | GET | /v1/user/bookmarks/novel | 200 | 0 | 0 | none | confirmed | confirmed | inconclusive | confirmed | confirmed | inconclusive | second_page_not_observed |
| user-novel-bookmarks-private | GET | /v1/user/bookmarks/novel | 200 | 0 | 0 | none | confirmed | confirmed | inconclusive | confirmed | confirmed | inconclusive | second_page_not_observed |
| novel-comments-v2 | GET | /v2/novel/comments | 200 | 0 | 0 | none | confirmed | confirmed | inconclusive | confirmed | confirmed | inconclusive | second_page_not_observed |
| novel-comments-v3 | GET | /v3/novel/comments | 200 | 0 | 0 | none | confirmed | confirmed | inconclusive | not_tested | not_tested | inconclusive | second_page_not_observed |
| search-illust-all | GET | /v1/search/illust | 200 | 30 | 30 | offset | confirmed | confirmed | confirmed | confirmed | confirmed | confirmed |  |
| search-illust-illust | GET | /v1/search/illust | 200 | 30 | 30 | offset | confirmed | confirmed | confirmed | confirmed | confirmed | confirmed |  |
| search-illust-manga | GET | /v1/search/illust | 200 | 30 | 30 | offset | confirmed | confirmed | confirmed | confirmed | confirmed | confirmed |  |
| search-illust-ugoira | GET | /v1/search/illust | 200 | 30 | 30 | offset | confirmed | confirmed | confirmed | confirmed | confirmed | confirmed |  |
| search-illust-rating | GET | /v1/search/illust | 200 | 30 | 30 | offset | confirmed | confirmed | confirmed | not_tested | not_tested | not_tested |  |
| illust-recommended | GET | /v1/illust/recommended | 200 | 85 | 0 | offset | confirmed | confirmed | inconclusive | confirmed | confirmed | inconclusive | second_page_error |
| illust-new | GET | /v1/illust/new | 200 | 30 | 30 | max_illust_id | confirmed | confirmed | confirmed | confirmed | confirmed | confirmed |  |
| illust-ranking | GET | /v1/illust/ranking | 200 | 30 | 30 | offset | confirmed | confirmed | confirmed | confirmed | confirmed | confirmed |  |
| user-illusts-illust | GET | /v1/user/illusts | 200 | 0 | 0 | none | confirmed | confirmed | inconclusive | confirmed | confirmed | inconclusive | second_page_not_observed |
| user-illusts-manga | GET | /v1/user/illusts | 200 | 0 | 0 | none | confirmed | confirmed | inconclusive | confirmed | confirmed | inconclusive | second_page_not_observed |
| user-illust-bookmarks-public | GET | /v1/user/bookmarks/illust | 200 | 16 | 0 | none | confirmed | confirmed | inconclusive | confirmed | confirmed | inconclusive | second_page_not_observed |
| user-illust-bookmarks-private | GET | /v1/user/bookmarks/illust | 200 | 0 | 0 | none | confirmed | confirmed | inconclusive | confirmed | confirmed | inconclusive | second_page_not_observed |
| illust-comments-v3 | GET | /v3/illust/comments | 200 | 0 | 0 | none | confirmed | confirmed | inconclusive | confirmed | confirmed | inconclusive | second_page_not_observed |
| ugoira-metadata | GET | /v1/ugoira/metadata | 200 | 0 | 0 | none | confirmed | confirmed | confirmed | confirmed | confirmed | confirmed |  |
| stamps | GET | /v1/stamps | 200 | 40 | 0 | none | confirmed | confirmed | confirmed | not_tested | not_tested | not_tested |  |
| illust-comment-text | POST | /v1/illust/comment/add | 0 | 0 | 0 | none | not_tested | not_tested | not_tested | not_tested | not_tested | not_tested | mutation_runner_pending |
| illust-comment-reply | POST | /v1/illust/comment/add | 0 | 0 | 0 | none | not_tested | not_tested | not_tested | not_tested | not_tested | not_tested | mutation_runner_pending |
| novel-comment-stamp | POST | /v1/novel/comment/add | 0 | 0 | 0 | none | not_tested | not_tested | not_tested | not_tested | not_tested | not_tested | mutation_runner_pending |
| novel-comment-text | POST | /v1/novel/comment/add | 0 | 0 | 0 | none | not_tested | not_tested | not_tested | not_tested | not_tested | not_tested | mutation_runner_pending |
| illust-comment-delete | POST | /v1/illust/comment/delete | 0 | 0 | 0 | none | not_tested | not_tested | not_tested | not_tested | not_tested | not_tested | mutation_runner_pending |
| novel-comment-delete | POST | /v1/novel/comment/delete | 0 | 0 | 0 | none | not_tested | not_tested | not_tested | not_tested | not_tested | not_tested | mutation_runner_pending |
