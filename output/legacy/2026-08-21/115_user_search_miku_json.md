# Search Pixiv users with a bounded result set

## Command
```shell
cd /private/tmp/pixiv-cli-e2e-shell-20260821 && env HOME=/private/tmp/pixiv-cli-e2e-shell-20260821/home ./bin/pixiv user search miku --limit 3 --json --proxy http://127.0.0.1:7890
```

## Output
```text
{
  "user_previews": [
    {
      "user": {
        "id": 7621567,
        "name": "Chomikuplus",
        "account": "chomiku",
        "comment": "",
        "is_followed": false,
        "profile_image": {
          "resource": {
            "ref": "eyJ2IjoxLCJwIjoicGl4aXYiLCJkIjoiZXlKcklqb2lkWE5sY2w5d2NtOW1hV3hsSWl3aWFXUWlPamMyTWpFMU5qY3NJbkFpT2kweExDSjJJam9pYldWa2FYVnRJbjA9In0"
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
        "id": 33039505,
        "name": "mikuning",
        "account": "user_ehur8484",
        "comment": "",
        "is_followed": false,
        "profile_image": {
          "resource": {
            "ref": "eyJ2IjoxLCJwIjoicGl4aXYiLCJkIjoiZXlKcklqb2lkWE5sY2w5d2NtOW1hV3hsSWl3aWFXUWlPak16TURNNU5UQTFMQ0p3SWpvdE1Td2lkaUk2SW0xbFpHbDFiU0o5In0"
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
        "id": 4262043,
        "name": "mikuma/MkMA",
        "account": "totamimo",
        "comment": "",
        "is_followed": false,
        "profile_image": {
          "resource": {
            "ref": "eyJ2IjoxLCJwIjoicGl4aXYiLCJkIjoiZXlKcklqb2lkWE5sY2w5d2NtOW1hV3hsSWl3aWFXUWlPalF5TmpJd05ETXNJbkFpT2kweExDSjJJam9pYldWa2FYVnRJbjA9In0"
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
