# Fetch authenticated Pixiv daily ranking with a bounded result set

## Command
```shell
cd /private/tmp/pixiv-cli-e2e-shell-20260821 && env HOME=/private/tmp/pixiv-cli-e2e-shell-20260821/home ./bin/pixiv ranking --mode day --limit 3 --json --proxy http://127.0.0.1:7890
```

## Output
```text
{
  "illusts": [
    {
      "id": 148687343,
      "title": "【会社と私生活】男の人",
      "caption": "最新5巻はこちらから\u003cbr /\u003e《通常版》\u003ca href=\"https://amzn.asia/d/iKiGiTa\" target='_blank' rel='noopener noreferrer'\u003ehttps://amzn.asia/d/iKiGiTa\u003c/a\u003e\u003cbr /\u003e《特装版（おまけ小冊子付）》\u003ca href=\"https://amzn.asia/d/cWKTl7Q\" target='_blank' rel='noopener noreferrer'\u003ehttps://amzn.asia/d/cWKTl7Q\u003c/a\u003e",
      "kind": "manga",
      "raw_kind": "manga",
      "tags": [
        {
          "name": "漫画",
          "translated_name": ""
        },
        {
          "name": "会社と私生活",
          "translated_name": ""
        },
        {
          "name": "はよ結婚しろ",
          "translated_name": ""
        }
      ],
      "user": {
        "id": 4729811,
        "name": "金沢真之介",
        "account": "koikanazawa",
        "comment": "",
        "is_followed": false,
        "profile_image": {
          "resource": {
            "ref": "eyJ2IjoxLCJwIjoicGl4aXYiLCJkIjoiZXlKcklqb2lkWE5sY2w5d2NtOW1hV3hsSWl3aWFXUWlPalEzTWprNE1URXNJbkFpT2kweExDSjJJam9pYldWa2FYVnRJbjA9In0"
          },
          "variant": "medium",
          "width": 0,
          "height": 0
        }
      },
      "published_at": "2026-08-21T03:00:26Z",
      "total_bookmarks": 4248,
      "total_views": 64843,
      "width": 3240,
      "height": 4050,
      "page_count": 4,
      "x_restrict": 0,
      "ai_type": 1,
      "cover": {
        "resource": {
          "ref": "eyJ2IjoxLCJwIjoicGl4aXYiLCJkIjoiZXlKcklqb2lZWEowZDI5eWF5SXNJbWxrSWpveE5EZzJPRGN6TkRNc0luQWlPaTB4TENKMklqb2liR0Z5WjJVaWZRPT0ifQ"
        },
        "variant": "large",
        "width": 3240,
        "height": 4050
      }
    },
    {
      "id": 148682428,
      "title": "真夏の夜のドリーム",
      "caption": "スイカ、キャンプ、ちいさい焚火、　釣り",
      "kind": "illustration",
      "raw_kind": "illust",
      "tags": [
        {
          "name": "創作",
          "translated_name": ""
        },
        {
          "name": "スイカ",
          "translated_name": ""
        },
        {
          "name": "夏",
          "translated_name": ""
        },
        {
          "name": "夏の夜の夢",
          "translated_name": ""
        }
      ],
      "user": {
        "id": 33333,
        "name": "ポ～ン（出水ぽすか）",
        "account": "pone",
        "comment": "",
        "is_followed": false,
        "profile_image": {
          "resource": {
            "ref": "eyJ2IjoxLCJwIjoicGl4aXYiLCJkIjoiZXlKcklqb2lkWE5sY2w5d2NtOW1hV3hsSWl3aWFXUWlPak16TXpNekxDSndJam90TVN3aWRpSTZJbTFsWkdsMWJTSjkifQ"
          },
          "variant": "medium",
          "width": 0,
          "height": 0
        }
      },
      "published_at": "2026-08-20T22:30:01Z",
      "total_bookmarks": 2759,
      "total_views": 18404,
      "width": 1400,
      "height": 991,
      "page_count": 1,
      "x_restrict": 0,
      "ai_type": 1,
      "cover": {
        "resource": {
          "ref": "eyJ2IjoxLCJwIjoicGl4aXYiLCJkIjoiZXlKcklqb2lZWEowZDI5eWF5SXNJbWxrSWpveE5EZzJPREkwTWpnc0luQWlPaTB4TENKMklqb2liR0Z5WjJVaWZRPT0ifQ"
        },
        "variant": "large",
        "width": 1400,
        "height": 991
      }
    },
    {
      "id": 148655700,
      "title": "わたしが恋人になれるわけないじゃん、ムリムリ！",
      "caption": "【重大発表】\u003cbr /\u003e\u003cbr /\u003eこの度、『わたしが恋人になれるわけないじゃん、ムリムリ！（※ムリじゃなかった!?）～セカンドシーズン～』（わたなれ）の\u003cbr /\u003eコミカライズを担当することになりました！やった〜～！\u003cbr /\u003e\u003cbr /\u003eまさかの引き継ぎ！\u003cbr /\u003e\u003cbr /\u003eわたしが担当するのは、原作5巻以降！\u003cbr /\u003e\u003cbr /\u003e原作4巻までは、むっしゅ先生の漫画版やアニメ＆映画で描かれている範囲なので、\u003cbr /\u003eこれまで漫画版やアニメ＆映画を楽しんでいた方にも、その続きとして入りやすい作品になっていると思います☝🏻💭\u003cbr /\u003e\u003cbr /\u003e連載開始は来年初春（1-2月）を予定しています。\u003cbr /\u003eぜひお楽しみに～！！\u003cbr /\u003e\u003cbr /\u003eプレッシャーもありますが、精いっぱい頑張ります。\u003cbr /\u003eよろしくお願いいたします！",
      "kind": "illustration",
      "raw_kind": "illust",
      "tags": [
        {
          "name": "百合",
          "translated_name": ""
        },
        {
          "name": "わたなれ",
          "translated_name": ""
        },
        {
          "name": "わたしが恋人になれるわけないじゃん、ムリムリ!",
          "translated_name": ""
        },
        {
          "name": "甘織れな子",
          "translated_name": ""
        },
        {
          "name": "王塚真唯",
          "translated_name": ""
        },
        {
          "name": "瀬名紫陽花",
          "translated_name": ""
        },
        {
          "name": "琴紗月",
          "translated_name": ""
        },
        {
          "name": "小柳香穂",
          "translated_name": ""
        },
        {
          "name": "クインテット(わたなれ)",
          "translated_name": ""
        },
        {
          "name": "わたなれ1000users入り",
          "translated_name": ""
        }
      ],
      "user": {
        "id": 54891433,
        "name": "工藤える",
        "account": "user_tjmm5342",
        "comment": "",
        "is_followed": false,
        "profile_image": {
          "resource": {
            "ref": "eyJ2IjoxLCJwIjoicGl4aXYiLCJkIjoiZXlKcklqb2lkWE5sY2w5d2NtOW1hV3hsSWl3aWFXUWlPalUwT0RreE5ETXpMQ0p3SWpvdE1Td2lkaUk2SW0xbFpHbDFiU0o5In0"
          },
          "variant": "medium",
          "width": 0,
          "height": 0
        }
      },
      "published_at": "2026-08-20T08:00:09Z",
      "total_bookmarks": 4925,
      "total_views": 28983,
      "width": 1449,
      "height": 2048,
      "page_count": 2,
      "x_restrict": 0,
      "ai_type": 1,
      "cover": {
        "resource": {
          "ref": "eyJ2IjoxLCJwIjoicGl4aXYiLCJkIjoiZXlKcklqb2lZWEowZDI5eWF5SXNJbWxrSWpveE5EZzJOVFUzTURBc0luQWlPaTB4TENKMklqb2liR0Z5WjJVaWZRPT0ifQ"
        },
        "variant": "large",
        "width": 1449,
        "height": 2048
      }
    }
  ]
}
```

Exit code: 0

Verdict: PASS
