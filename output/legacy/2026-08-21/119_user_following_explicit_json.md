# List users followed by a public Pixiv user

## Command
```shell
cd /private/tmp/pixiv-cli-e2e-shell-20260821 && env HOME=/private/tmp/pixiv-cli-e2e-shell-20260821/home ./bin/pixiv user following 7621567 --restrict public --limit 3 --json --proxy http://127.0.0.1:7890
```

## Output
```text
{
  "user_previews": [
    {
      "user": {
        "id": 97691537,
        "name": "びふみる",
        "account": "vfmelu",
        "comment": "",
        "is_followed": false,
        "profile_image": {
          "resource": {
            "ref": "eyJ2IjoxLCJwIjoicGl4aXYiLCJkIjoiZXlKcklqb2lkWE5sY2w5d2NtOW1hV3hsSWl3aWFXUWlPamszTmpreE5UTTNMQ0p3SWpvdE1Td2lkaUk2SW0xbFpHbDFiU0o5In0"
          },
          "variant": "medium",
          "width": 0,
          "height": 0
        }
      },
      "illusts": [],
      "novels": []
    },
    {
      "user": {
        "id": 492048,
        "name": "sb",
        "account": "coco1",
        "comment": "",
        "is_followed": true,
        "profile_image": {
          "resource": {
            "ref": "eyJ2IjoxLCJwIjoicGl4aXYiLCJkIjoiZXlKcklqb2lkWE5sY2w5d2NtOW1hV3hsSWl3aWFXUWlPalE1TWpBME9Dd2ljQ0k2TFRFc0luWWlPaUp0WldScGRXMGlmUT09In0"
          },
          "variant": "medium",
          "width": 0,
          "height": 0
        }
      },
      "illusts": [],
      "novels": []
    },
    {
      "user": {
        "id": 7375141,
        "name": "MiSuDo",
        "account": "misudo_ask",
        "comment": "",
        "is_followed": false,
        "profile_image": {
          "resource": {
            "ref": "eyJ2IjoxLCJwIjoicGl4aXYiLCJkIjoiZXlKcklqb2lkWE5sY2w5d2NtOW1hV3hsSWl3aWFXUWlPamN6TnpVeE5ERXNJbkFpT2kweExDSjJJam9pYldWa2FYVnRJbjA9In0"
          },
          "variant": "medium",
          "width": 0,
          "height": 0
        }
      },
      "illusts": [],
      "novels": []
    }
  ]
}
```

Exit code: 0

Verdict: PASS
