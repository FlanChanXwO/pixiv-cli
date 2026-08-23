# Fetch personalized Pixiv artwork recommendations

## Command
```shell
cd /private/tmp/pixiv-cli-e2e-shell-20260821 && env HOME=/private/tmp/pixiv-cli-e2e-shell-20260821/home ./bin/pixiv recommended --type artwork --content-type illust --limit 3 --json --proxy http://127.0.0.1:7890
```

## Output
```text
{
  "illusts": [
    {
      "id": 123291149,
      "title": "Camila on the bed sheets",
      "caption": "fan art",
      "kind": "illustration",
      "raw_kind": "illust",
      "tags": [
        {
          "name": "女の子",
          "translated_name": ""
        },
        {
          "name": "VTuber",
          "translated_name": ""
        },
        {
          "name": "バーチャルYouTuber",
          "translated_name": ""
        },
        {
          "name": "ENVTuber",
          "translated_name": ""
        },
        {
          "name": "Camila",
          "translated_name": ""
        }
      ],
      "user": {
        "id": 3156515,
        "name": "Satokon",
        "account": "satokon415",
        "comment": "",
        "is_followed": false,
        "profile_image": {
          "resource": {
            "ref": "eyJ2IjoxLCJwIjoicGl4aXYiLCJkIjoiZXlKcklqb2lkWE5sY2w5d2NtOW1hV3hsSWl3aWFXUWlPak14TlRZMU1UVXNJbkFpT2kweExDSjJJam9pYldWa2FYVnRJbjA9In0"
          },
          "variant": "medium",
          "width": 0,
          "height": 0
        }
      },
      "published_at": "2024-10-13T06:48:52Z",
      "total_bookmarks": 336,
      "total_views": 1816,
      "width": 2230,
      "height": 3541,
      "page_count": 1,
      "x_restrict": 0,
      "ai_type": 1,
      "tools": [
        "SAI"
      ],
      "cover": {
        "resource": {
          "ref": "eyJ2IjoxLCJwIjoicGl4aXYiLCJkIjoiZXlKcklqb2lZWEowZDI5eWF5SXNJbWxrSWpveE1qTXlPVEV4TkRrc0luQWlPaTB4TENKMklqb2liR0Z5WjJVaWZRPT0ifQ"
        },
        "variant": "large",
        "width": 2230,
        "height": 3541
      }
    },
    {
      "id": 148684691,
      "title": "Daisy",
      "caption": "",
      "kind": "illustration",
      "raw_kind": "illust",
      "tags": [
        {
          "name": "ロリ",
          "translated_name": ""
        },
        {
          "name": "中世",
          "translated_name": ""
        },
        {
          "name": "エルフ",
          "translated_name": ""
        }
      ],
      "user": {
        "id": 100799686,
        "name": "mashigirls",
        "account": "user_enxg7433",
        "comment": "",
        "is_followed": false,
        "profile_image": {
          "resource": {
            "ref": "eyJ2IjoxLCJwIjoicGl4aXYiLCJkIjoiZXlKcklqb2lkWE5sY2w5d2NtOW1hV3hsSWl3aWFXUWlPakV3TURjNU9UWTROaXdpY0NJNkxURXNJbllpT2lKdFpXUnBkVzBpZlE9PSJ9"
          },
          "variant": "medium",
          "width": 0,
          "height": 0
        }
      },
      "published_at": "2026-08-21T00:44:52Z",
      "total_bookmarks": 43,
      "total_views": 211,
      "width": 2063,
      "height": 3000,
      "page_count": 1,
      "x_restrict": 0,
      "ai_type": 1,
      "cover": {
        "resource": {
          "ref": "eyJ2IjoxLCJwIjoicGl4aXYiLCJkIjoiZXlKcklqb2lZWEowZDI5eWF5SXNJbWxrSWpveE5EZzJPRFEyT1RFc0luQWlPaTB4TENKMklqb2liR0Z5WjJVaWZRPT0ifQ"
        },
        "variant": "large",
        "width": 2063,
        "height": 3000
      }
    },
    {
      "id": 148646679,
      "title": "Cheyanne",
      "caption": "",
      "kind": "illustration",
      "raw_kind": "illust",
      "tags": [
        {
          "name": "女の子",
          "translated_name": ""
        },
        {
          "name": "ドールズフロントライン",
          "translated_name": ""
        },
        {
          "name": "ドールズフロントライン2",
          "translated_name": ""
        },
        {
          "name": "少女前線",
          "translated_name": ""
        },
        {
          "name": "少前2:追放",
          "translated_name": ""
        },
        {
          "name": "Cheyanne",
          "translated_name": ""
        },
        {
          "name": "M200(ドールズフロントライン)",
          "translated_name": ""
        },
        {
          "name": "シャイアン",
          "translated_name": ""
        },
        {
          "name": "M200",
          "translated_name": ""
        }
      ],
      "user": {
        "id": 105360325,
        "name": "ChoPingme",
        "account": "user_kzdg4242",
        "comment": "",
        "is_followed": false,
        "profile_image": {
          "resource": {
            "ref": "eyJ2IjoxLCJwIjoicGl4aXYiLCJkIjoiZXlKcklqb2lkWE5sY2w5d2NtOW1hV3hsSWl3aWFXUWlPakV3TlRNMk1ETXlOU3dpY0NJNkxURXNJbllpT2lKdFpXUnBkVzBpZlE9PSJ9"
          },
          "variant": "medium",
          "width": 0,
          "height": 0
        }
      },
      "published_at": "2026-08-20T00:11:07Z",
      "total_bookmarks": 319,
      "total_views": 943,
      "width": 1549,
      "height": 978,
      "page_count": 1,
      "x_restrict": 0,
      "ai_type": 1,
      "tools": [
        "CLIP STUDIO PAINT"
      ],
      "cover": {
        "resource": {
          "ref": "eyJ2IjoxLCJwIjoicGl4aXYiLCJkIjoiZXlKcklqb2lZWEowZDI5eWF5SXNJbWxrSWpveE5EZzJORFkyTnprc0luQWlPaTB4TENKMklqb2liR0Z5WjJVaWZRPT0ifQ"
        },
        "variant": "large",
        "width": 1549,
        "height": 978
      }
    }
  ]
}
```

Exit code: 0

Verdict: PASS
