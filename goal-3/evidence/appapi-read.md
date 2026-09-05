# Pixiv App API legacy read evidence

观测日期：2026-09-05（Asia/Shanghai）。

本文件保留原有 85 条记录。

重新分级规则：

- 原 `confirmed` 基础 HTTP 记录改为 `inconclusive`。
- 原 `ignored` 改为 `not_tested`。
- 既有 stamp mutation read-back 记录保留 `confirmed`。
- 这些记录不含 token、cookie、UID、用户名、正文、标题或原始 URL。

| 序号 | Case | Method | Path | Status | Items | Continuation | Verdict | Reclassification |
| ---: | --- | --- | --- | ---: | ---: | --- | --- | --- |
| 1 | search-illust-content-type | GET | /v1/search/illust | 200 | 30 | yes | inconclusive | legacy_partial_http_evidence |
| 2 | search-rating-contract | GET | /v1/search/illust | 200 | 30 | yes | inconclusive | legacy_partial_http_evidence |
| 3 | recommended-content-type | GET | /v1/illust/recommended | 200 | 88 | yes | inconclusive | legacy_partial_http_evidence |
| 4 | illust-new-content-type | GET | /v1/illust/new | 200 | 29 | yes | inconclusive | legacy_partial_http_evidence |
| 5 | novel-new-pagination | GET | /v1/novel/new | 200 | 30 | yes | inconclusive | legacy_partial_http_evidence |
| 6 | novel-detail | GET | /v2/novel/detail | 200 | 0 | no | inconclusive | legacy_partial_http_evidence |
| 7 | novel-series | GET | /v2/novel/series | 0 | 0 | no | inconclusive | historical |
| 8 | novel-content | GET | /v1/novel/content | 0 | 0 | no | inconclusive | historical |
| 9 | search-novel-period | GET | /v1/search/novel | 200 | 30 | yes | inconclusive | legacy_partial_http_evidence |
| 10 | novel-bookmark-read | GET | /v1/user/bookmarks/novel | 0 | 0 | no | inconclusive | historical |
| 11 | artwork-bookmark-subtype | GET | /v1/user/bookmarks/illust | 0 | 0 | no | inconclusive | historical |
| 12 | comments-total | GET | /v3/illust/comments | 200 | 0 | yes | inconclusive | legacy_partial_http_evidence |
| 13 | comment-mutation-contract | POST | /v1/illust/comment/add | 0 | 0 | no | not_tested | legacy_not_tested |
| 14 | novel-ranking-contract | GET | /v1/novel/ranking | 0 | 0 | no | inconclusive | historical |
| 15 | search-illust-content-type | GET | /v1/search/illust | 200 | 30 | yes | inconclusive | legacy_partial_http_evidence |
| 16 | search-rating-contract | GET | /v1/search/illust | 200 | 30 | yes | inconclusive | legacy_partial_http_evidence |
| 17 | recommended-content-type | GET | /v1/illust/recommended | 200 | 86 | yes | inconclusive | legacy_partial_http_evidence |
| 18 | illust-new-content-type | GET | /v1/illust/new | 200 | 29 | yes | inconclusive | legacy_partial_http_evidence |
| 19 | novel-new-pagination | GET | /v1/novel/new | 200 | 30 | yes | inconclusive | legacy_partial_http_evidence |
| 20 | novel-detail | GET | /v2/novel/detail | 200 | 0 | no | inconclusive | legacy_partial_http_evidence |
| 21 | novel-series | GET | /v2/novel/series | 0 | 0 | no | inconclusive | historical |
| 22 | novel-content | GET | /v1/novel/content | 0 | 0 | no | inconclusive | historical |
| 23 | search-novel-period | GET | /v1/search/novel | 200 | 30 | yes | inconclusive | legacy_partial_http_evidence |
| 24 | novel-bookmark-read | GET | /v1/user/bookmarks/novel | 0 | 0 | no | inconclusive | historical |
| 25 | artwork-bookmark-subtype | GET | /v1/user/bookmarks/illust | 0 | 0 | no | inconclusive | historical |
| 26 | comments-total | GET | /v3/illust/comments | 200 | 0 | yes | inconclusive | legacy_partial_http_evidence |
| 27 | comment-mutation-contract | POST | /v1/illust/comment/add | 0 | 0 | no | not_tested | legacy_not_tested |
| 28 | novel-ranking-contract | GET | /v1/novel/ranking | 0 | 0 | no | inconclusive | historical |
| 29 | search-illust-content-type | GET | /v1/search/illust | 200 | 30 | yes | inconclusive | legacy_partial_http_evidence |
| 30 | search-rating-contract | GET | /v1/search/illust | 200 | 30 | yes | inconclusive | legacy_partial_http_evidence |
| 31 | recommended-content-type | GET | /v1/illust/recommended | 200 | 85 | yes | inconclusive | legacy_partial_http_evidence |
| 32 | illust-new-content-type | GET | /v1/illust/new | 200 | 29 | yes | inconclusive | legacy_partial_http_evidence |
| 33 | novel-new-pagination | GET | /v1/novel/new | 200 | 30 | yes | inconclusive | legacy_partial_http_evidence |
| 34 | novel-detail | GET | /v2/novel/detail | 200 | 0 | no | inconclusive | legacy_partial_http_evidence |
| 35 | novel-series | GET | /v2/novel/series | 0 | 0 | no | inconclusive | historical |
| 36 | novel-content | GET | /v1/novel/content | 0 | 0 | no | inconclusive | historical |
| 37 | search-novel-period | GET | /v1/search/novel | 200 | 30 | yes | inconclusive | legacy_partial_http_evidence |
| 38 | novel-bookmark-read | GET | /v1/user/bookmarks/novel | 0 | 0 | no | inconclusive | historical |
| 39 | artwork-bookmark-subtype | GET | /v1/user/bookmarks/illust | 0 | 0 | no | inconclusive | historical |
| 40 | comments-total | GET | /v3/illust/comments | 200 | 0 | yes | inconclusive | legacy_partial_http_evidence |
| 41 | comment-mutation-contract | POST | /v1/illust/comment/add | 0 | 0 | no | not_tested | legacy_not_tested |
| 42 | novel-ranking-contract | GET | /v1/novel/ranking | 0 | 0 | no | inconclusive | historical |
| 43 | search-illust-content-type | GET | /v1/search/illust | 200 | 30 | yes | inconclusive | legacy_partial_http_evidence |
| 44 | search-rating-contract | GET | /v1/search/illust | 200 | 30 | yes | inconclusive | legacy_partial_http_evidence |
| 45 | recommended-content-type | GET | /v1/illust/recommended | 200 | 84 | yes | inconclusive | legacy_partial_http_evidence |
| 46 | illust-new-content-type | GET | /v1/illust/new | 200 | 29 | yes | inconclusive | legacy_partial_http_evidence |
| 47 | novel-new-pagination | GET | /v1/novel/new | 200 | 30 | yes | inconclusive | legacy_partial_http_evidence |
| 48 | novel-detail | GET | /v2/novel/detail | 200 | 0 | no | inconclusive | legacy_partial_http_evidence |
| 49 | novel-series | GET | /v2/novel/series | 0 | 0 | no | inconclusive | historical |
| 50 | novel-content | GET | /v1/novel/content | 0 | 0 | no | inconclusive | historical |
| 51 | search-novel-period | GET | /v1/search/novel | 200 | 30 | yes | inconclusive | legacy_partial_http_evidence |
| 52 | novel-bookmark-read | GET | /v1/user/bookmarks/novel | 0 | 0 | no | inconclusive | historical |
| 53 | artwork-bookmark-subtype | GET | /v1/user/bookmarks/illust | 0 | 0 | no | inconclusive | historical |
| 54 | comments-total | GET | /v3/illust/comments | 200 | 0 | yes | inconclusive | legacy_partial_http_evidence |
| 55 | comment-mutation-contract | POST | /v1/illust/comment/add | 0 | 0 | no | not_tested | legacy_not_tested |
| 56 | novel-ranking-contract | GET | /v1/novel/ranking | 0 | 0 | no | inconclusive | historical |
| 57 | search-illust-content-type | GET | /v1/search/illust | 200 | 30 | yes | inconclusive | legacy_partial_http_evidence |
| 58 | search-rating-contract | GET | /v1/search/illust | 200 | 30 | yes | inconclusive | legacy_partial_http_evidence |
| 59 | recommended-content-type | GET | /v1/illust/recommended | 200 | 89 | yes | inconclusive | legacy_partial_http_evidence |
| 60 | illust-new-content-type | GET | /v1/illust/new | 200 | 29 | yes | inconclusive | legacy_partial_http_evidence |
| 61 | novel-new-pagination | GET | /v1/novel/new | 200 | 30 | yes | inconclusive | legacy_partial_http_evidence |
| 62 | novel-detail | GET | /v2/novel/detail | 200 | 0 | no | inconclusive | legacy_partial_http_evidence |
| 63 | novel-series | GET | /v2/novel/series | 0 | 0 | no | inconclusive | historical |
| 64 | novel-content | GET | /v1/novel/content | 0 | 0 | no | inconclusive | historical |
| 65 | search-novel-period | GET | /v1/search/novel | 200 | 30 | yes | inconclusive | legacy_partial_http_evidence |
| 66 | novel-bookmark-read | GET | /v1/user/bookmarks/novel | 0 | 0 | no | inconclusive | historical |
| 67 | artwork-bookmark-subtype | GET | /v1/user/bookmarks/illust | 0 | 0 | no | inconclusive | historical |
| 68 | comments-total | GET | /v3/illust/comments | 200 | 0 | yes | inconclusive | legacy_partial_http_evidence |
| 69 | comment-mutation-contract | POST | /v1/illust/comment/add | 0 | 0 | no | not_tested | legacy_not_tested |
| 70 | novel-ranking-contract | GET | /v1/novel/ranking | 0 | 0 | no | inconclusive | historical |
| 71 | search-illust-content-type | GET | /v1/search/illust | 200 | 30 | yes | inconclusive | legacy_partial_http_evidence |
| 72 | search-rating-contract | GET | /v1/search/illust | 200 | 30 | yes | inconclusive | legacy_partial_http_evidence |
| 73 | recommended-content-type | GET | /v1/illust/recommended | 200 | 86 | yes | inconclusive | legacy_partial_http_evidence |
| 74 | illust-new-content-type | GET | /v1/illust/new | 200 | 29 | yes | inconclusive | legacy_partial_http_evidence |
| 75 | novel-new-pagination | GET | /v1/novel/new | 200 | 30 | yes | inconclusive | legacy_partial_http_evidence |
| 76 | novel-detail | GET | /v2/novel/detail | 200 | 0 | no | inconclusive | legacy_partial_http_evidence |
| 77 | novel-series | GET | /v2/novel/series | 0 | 0 | no | inconclusive | historical |
| 78 | novel-content | GET | /v1/novel/content | 0 | 0 | no | inconclusive | historical |
| 79 | search-novel-period | GET | /v1/search/novel | 200 | 30 | yes | inconclusive | legacy_partial_http_evidence |
| 80 | novel-bookmark-read | GET | /v1/user/bookmarks/novel | 0 | 0 | no | inconclusive | historical |
| 81 | artwork-bookmark-subtype | GET | /v1/user/bookmarks/illust | 0 | 0 | no | inconclusive | historical |
| 82 | comments-total | GET | /v3/illust/comments | 200 | 0 | yes | inconclusive | legacy_partial_http_evidence |
| 83 | comment-mutation-contract | POST | /v1/illust/comment/add | 0 | 0 | no | not_tested | legacy_not_tested |
| 84 | novel-ranking-contract | GET | /v1/novel/ranking | 0 | 0 | no | inconclusive | historical |
| 85 | comment-stamp-write | POST | /v1/illust/comment/add | 200 | 1 | no | confirmed | live_mutation_readback_confirmed |
