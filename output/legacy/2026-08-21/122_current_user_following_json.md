# List current authenticated user following using omitted USER_ID

## Command
```shell
cd /private/tmp/pixiv-cli-e2e-shell-20260821 && env HOME=/private/tmp/pixiv-cli-e2e-shell-20260821/home ./bin/pixiv user following --restrict public --limit 3 --json --proxy http://127.0.0.1:7890
```

## Output
```text
{
  "user_previews": [
    {
      "user": {
        "id": 4399829,
        "name": "LeeMaeHyang",
        "account": "loveblend",
        "comment": "",
        "is_followed": true,
        "profile_image": {
          "resource": {
            "ref": "eyJ2IjoxLCJwIjoicGl4aXYiLCJkIjoiZXlKcklqb2lkWE5sY2w5d2NtOW1hV3hsSWl3aWFXUWlPalF6T1RrNE1qa3NJbkFpT2kweExDSjJJam9pYldWa2FYVnRJbjA9In0"
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
        "id": 58440630,
        "name": "Myuchiron",
        "account": "myuchiron",
        "comment": "",
        "is_followed": true,
        "profile_image": {
          "resource": {
            "ref": "eyJ2IjoxLCJwIjoicGl4aXYiLCJkIjoiZXlKcklqb2lkWE5sY2w5d2NtOW1hV3hsSWl3aWFXUWlPalU0TkRRd05qTXdMQ0p3SWpvdE1Td2lkaUk2SW0xbFpHbDFiU0o5In0"
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
        "id": 4272325,
        "name": "セニフ",
        "account": "azunakano1111",
        "comment": "",
        "is_followed": true,
        "profile_image": {
          "resource": {
            "ref": "eyJ2IjoxLCJwIjoicGl4aXYiLCJkIjoiZXlKcklqb2lkWE5sY2w5d2NtOW1hV3hsSWl3aWFXUWlPalF5TnpJek1qVXNJbkFpT2kweExDSjJJam9pYldWa2FYVnRJbjA9In0"
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
