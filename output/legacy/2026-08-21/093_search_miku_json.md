# Search Pixiv illustrations with a bounded result set

## Command
```shell
cd /private/tmp/pixiv-cli-e2e-shell-20260821 && env HOME=/private/tmp/pixiv-cli-e2e-shell-20260821/home ./bin/pixiv search miku --type artwork --content-type illust --limit 3 --json --proxy http://127.0.0.1:7890
```

## Output
```text
{
  "illusts": [
    {
      "id": 148767434,
      "title": "Vocaloid: MILA THE JAGUAR 💚💜🎵",
      "caption": "Mila the Jaguar 🇺🇲\u003cbr /\u003eMila a Onça-Pintada 🇧🇷\u003cbr /\u003eジャガーのミラ 🇯🇵\u003cbr /\u003e\u003cbr /\u003eHey everyone\u0026#44; I have a YouTube channel called TataBlue2004. There you can see some of my work; I do digital art and speed painting... if you like my work\u0026#44; subscribe\u0026#44; give it lots of likes\u0026#44; and turn on notifications for my channel. \u003cbr /\u003e🧩🧩🧩🧩🧩🧩🧩🧩🧩🧩🧩🧩🧩🧩🧩🧩🧩\u003cbr /\u003e💚💜🎵💚💜🎵💚💜🎵💚💜🎵💚💜🎵💚💜🐆🐆🐆🐆🐆🐆🐆🐆🐆🐆🐆🐆🐆🐆🐆🐆🐆🎤🎤🎤🎤🎤🎤🎤🎤🎤🎤🎤🎤🎤🎤🎤🎤🎤",
      "kind": "illustration",
      "raw_kind": "illust",
      "tags": [
        {
          "name": "TataBlue2004",
          "translated_name": ""
        },
        {
          "name": "MilatheJaguar",
          "translated_name": ""
        },
        {
          "name": "Jaguar",
          "translated_name": ""
        },
        {
          "name": "Vocaloid",
          "translated_name": ""
        },
        {
          "name": "sonicthehedgehog",
          "translated_name": ""
        },
        {
          "name": "sonicfanart",
          "translated_name": ""
        },
        {
          "name": "HatsuneMiku",
          "translated_name": ""
        },
        {
          "name": "pripara",
          "translated_name": ""
        },
        {
          "name": "kawaiiart",
          "translated_name": ""
        },
        {
          "name": "fanart",
          "translated_name": ""
        }
      ],
      "user": {
        "id": 91349036,
        "name": "🎗TataBlue2004🧩",
        "account": "user_twge7384",
        "comment": "",
        "is_followed": false,
        "profile_image": {
          "resource": {
            "ref": "eyJ2IjoxLCJwIjoicGl4aXYiLCJkIjoiZXlKcklqb2lkWE5sY2w5d2NtOW1hV3hsSWl3aWFXUWlPamt4TXpRNU1ETTJMQ0p3SWpvdE1Td2lkaUk2SW0xbFpHbDFiU0o5In0"
          },
          "variant": "medium",
          "width": 0,
          "height": 0
        }
      },
      "published_at": "2026-08-22T20:28:19Z",
      "total_bookmarks": 0,
      "total_views": 3,
      "width": 1080,
      "height": 1440,
      "page_count": 2,
      "x_restrict": 0,
      "ai_type": 1,
      "cover": {
        "resource": {
          "ref": "eyJ2IjoxLCJwIjoicGl4aXYiLCJkIjoiZXlKcklqb2lZWEowZDI5eWF5SXNJbWxrSWpveE5EZzNOamMwTXpRc0luQWlPaTB4TENKMklqb2liR0Z5WjJVaWZRPT0ifQ"
        },
        "variant": "large",
        "width": 1080,
        "height": 1440
      }
    },
    {
      "id": 148767320,
      "title": "Vocaloid: MILA THE JAGUAR 💚🎵",
      "caption": "Mila the Jaguar 🇺🇲\u003cbr /\u003eMila a Onça-Pintada 🇧🇷\u003cbr /\u003eジャガーのミラ 🇯🇵\u003cbr /\u003e\u003cbr /\u003eHey everyone\u0026#44; I have a YouTube channel called TataBlue2004. There you can see some of my work; I do digital art and speed painting... if you like my work\u0026#44; subscribe\u0026#44; give it lots of likes\u0026#44; and turn on notifications for my channel. \u003cbr /\u003e🧩🧩🧩🧩🧩🧩🧩🧩🧩🧩🧩🧩🧩🧩🧩🧩🧩\u003cbr /\u003e💚🎵💚🎵💚🎵💚🎵💚🎵💚🎵💚🎵💚🎵💚🐆🐆🐆🐆🐆🐆🐆🐆🐆🐆🐆🐆🐆🐆🐆🐆🐆🎤🎤🎤🎤🎤🎤🎤🎤🎤🎤🎤🎤🎤🎤🎤🎤🎤",
      "kind": "illustration",
      "raw_kind": "illust",
      "tags": [
        {
          "name": "TataBlue2004",
          "translated_name": ""
        },
        {
          "name": "MilatheJaguar",
          "translated_name": ""
        },
        {
          "name": "Jaguar",
          "translated_name": ""
        },
        {
          "name": "sonicthehedgehog",
          "translated_name": ""
        },
        {
          "name": "sonicfanart",
          "translated_name": ""
        },
        {
          "name": "Vocaloid",
          "translated_name": ""
        },
        {
          "name": "HatsuneMiku",
          "translated_name": ""
        },
        {
          "name": "fanart",
          "translated_name": ""
        },
        {
          "name": "pripara",
          "translated_name": ""
        },
        {
          "name": "kawaiiart",
          "translated_name": ""
        }
      ],
      "user": {
        "id": 91349036,
        "name": "🎗TataBlue2004🧩",
        "account": "user_twge7384",
        "comment": "",
        "is_followed": false,
        "profile_image": {
          "resource": {
            "ref": "eyJ2IjoxLCJwIjoicGl4aXYiLCJkIjoiZXlKcklqb2lkWE5sY2w5d2NtOW1hV3hsSWl3aWFXUWlPamt4TXpRNU1ETTJMQ0p3SWpvdE1Td2lkaUk2SW0xbFpHbDFiU0o5In0"
          },
          "variant": "medium",
          "width": 0,
          "height": 0
        }
      },
      "published_at": "2026-08-22T20:18:46Z",
      "total_bookmarks": 0,
      "total_views": 4,
      "width": 1080,
      "height": 1440,
      "page_count": 2,
      "x_restrict": 0,
      "ai_type": 1,
      "cover": {
        "resource": {
          "ref": "eyJ2IjoxLCJwIjoicGl4aXYiLCJkIjoiZXlKcklqb2lZWEowZDI5eWF5SXNJbWxrSWpveE5EZzNOamN6TWpBc0luQWlPaTB4TENKMklqb2liR0Z5WjJVaWZRPT0ifQ"
        },
        "variant": "large",
        "width": 1080,
        "height": 1440
      }
    },
    {
      "id": 148767165,
      "title": "NaruCamie016",
      "caption": "\u0026quot;Underwater Kiss\u0026quot;\u003cbr /\u003e\u003cbr /\u003eMMD Models Provided by TBOMB200\u003cbr /\u003eFollow me on Twitter\u003cbr /\u003e@imwhoop_5321",
      "kind": "illustration",
      "raw_kind": "illust",
      "tags": [
        {
          "name": "R-18",
          "translated_name": ""
        },
        {
          "name": "NARUTO",
          "translated_name": ""
        },
        {
          "name": "MyHeroAcademia",
          "translated_name": ""
        },
        {
          "name": "うずまきナルト",
          "translated_name": ""
        },
        {
          "name": "現見・ケミィ",
          "translated_name": ""
        },
        {
          "name": "anime",
          "translated_name": ""
        },
        {
          "name": "sexy",
          "translated_name": ""
        },
        {
          "name": "MikuMikuDance",
          "translated_name": ""
        }
      ],
      "user": {
        "id": 127109964,
        "name": "ImWhoop!",
        "account": "user_pkxx5273",
        "comment": "",
        "is_followed": false,
        "profile_image": {
          "resource": {
            "ref": "eyJ2IjoxLCJwIjoicGl4aXYiLCJkIjoiZXlKcklqb2lkWE5sY2w5d2NtOW1hV3hsSWl3aWFXUWlPakV5TnpFd09UazJOQ3dpY0NJNkxURXNJbllpT2lKdFpXUnBkVzBpZlE9PSJ9"
          },
          "variant": "medium",
          "width": 0,
          "height": 0
        }
      },
      "published_at": "2026-08-22T20:06:10Z",
      "total_bookmarks": 1,
      "total_views": 9,
      "width": 1440,
      "height": 2560,
      "page_count": 1,
      "x_restrict": 1,
      "ai_type": 1,
      "cover": {
        "resource": {
          "ref": "eyJ2IjoxLCJwIjoicGl4aXYiLCJkIjoiZXlKcklqb2lZWEowZDI5eWF5SXNJbWxrSWpveE5EZzNOamN4TmpVc0luQWlPaTB4TENKMklqb2liR0Z5WjJVaWZRPT0ifQ"
        },
        "variant": "large",
        "width": 1440,
        "height": 2560
      }
    }
  ]
}
```

Exit code: 0

Verdict: PASS
