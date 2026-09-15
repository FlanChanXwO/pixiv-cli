# Pixiv App API strict upstream evidence

| Case | Method | Path | HTTP | Page 1 | Page 2 | Continuation | Wire | Response | Pagination | Adapter | SDK | Verdict | Failure |
| --- | --- | --- | ---: | ---: | ---: | --- | --- | --- | --- | --- | --- | --- | --- |
| illust-comment-text | POST | /v1/illust/comment/add | 200 | 1 | 1 | none | confirmed | confirmed | confirmed | not_tested | not_tested | not_tested |  |
| illust-comment-reply | POST | /v1/illust/comment/add | 200 | 1 | 1 | none | confirmed | confirmed | confirmed | not_tested | not_tested | not_tested |  |
| novel-comment-stamp | POST | /v1/novel/comment/add | 200 | 1 | 1 | none | confirmed | confirmed | confirmed | not_tested | not_tested | not_tested |  |
| novel-comment-text | POST | /v1/novel/comment/add | 200 | 1 | 1 | none | confirmed | confirmed | confirmed | not_tested | not_tested | not_tested |  |
| illust-comment-delete | POST | /v1/illust/comment/delete | 200 | 1 | 1 | none | confirmed | confirmed | confirmed | not_tested | not_tested | not_tested |  |
| novel-comment-delete | POST | /v1/novel/comment/delete | 200 | 1 | 1 | none | confirmed | confirmed | confirmed | not_tested | not_tested | not_tested |  |
