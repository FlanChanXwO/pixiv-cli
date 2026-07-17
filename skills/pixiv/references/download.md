# Download workflows

Downloads write to disk. First run `pixiv config get download_path`, confirm
that directory and the item count, then execute the requested download.

```text
pixiv download 129543211
pixiv download 129543211 130000001
```

- Multi-page works download all pages automatically.
- Ugoira downloads use the built-in Rust encoder for GIF/APNG and can take
  time. Do not invent a timeout or terminate a healthy encode.
- Anonymous sessions may download public works via Web fallback. Restricted
  works can require App authentication; surface the real error rather than
  retrying through a Cookie or another hidden route.
- Report each requested ID's actual result. Do not summarize partial failures
  as success.
