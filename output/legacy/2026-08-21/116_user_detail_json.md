# Fetch Pixiv public user detail from a search result

## Command
```shell
cd /private/tmp/pixiv-cli-e2e-shell-20260821 && env HOME=/private/tmp/pixiv-cli-e2e-shell-20260821/home ./bin/pixiv user detail 7621567 --json --proxy http://127.0.0.1:7890
```

## Output
```text
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
  "profile": {
    "webpage": "",
    "gender": "",
    "birth_day": "02-01",
    "birth_year": 1997,
    "region": "",
    "country_code": "",
    "job": "大学生・院生",
    "total_follow_users": 1889,
    "total_my_pixiv_users": 8,
    "total_illusts": 658,
    "total_manga": 0,
    "total_novels": 0,
    "total_illust_bookmarks": 334,
    "total_illust_series": 0,
    "total_novel_series": 0,
    "background_image_url": "https://i.pximg.net/c/1200x600_90_a2_g5/background/img/2026/03/18/02/14/47/7621567_5b884236c1587bcf84ab34507d7d890e_master1200.jpg",
    "twitter_account": "",
    "twitter_url": "",
    "pawoo_url": "https://pawoo.net/oauth_authentications/7621567?provider=pixiv",
    "is_premium": true,
    "is_using_custom_profile_image": true
  },
  "profile_publicity": {
    "gender": true,
    "region": true,
    "birth_day": true,
    "birth_year": true,
    "job": true,
    "pawoo": true
  },
  "workspace": {
    "pc": "",
    "monitor": "",
    "tool": "",
    "scanner": "",
    "tablet": "",
    "mouse": "",
    "printer": "",
    "desktop": "",
    "music": "",
    "desk": "",
    "chair": "",
    "comment": "",
    "workspace_image_url": ""
  }
}
```

Exit code: 0

Verdict: PASS
