## Summary

<!-- Describe the user problem and the change that addresses it. Link related issues with "Fixes #123" when applicable. -->

## Scope and compatibility

<!-- List affected CLI commands or flags, MCP tools or schemas, SDK APIs, configuration, environment variables, output contracts, and release behavior. State "None" when there is no public impact. -->

## Verification

<!-- List the exact commands you ran and their results. For real Pixiv API coverage, state whether it was run and use only redacted evidence. -->

```text
go test ./...
```

## Checklist

- [ ] The change is focused and linked to an issue when appropriate.
- [ ] I added or updated focused tests for changed behavior.
- [ ] I ran the relevant tests and recorded the results above.
- [ ] I updated the required CLI, MCP, SDK, README, maintainer, and product-skill documentation.
- [ ] I updated both unreleased changelog files for user-visible changes, compatibility effects, removals, or security effects.
- [ ] I documented every new timeout, retry, pagination or result limit, truncation, fallback, or downgrade and its evidence.
- [ ] I did not add refresh tokens, cookies, authorization codes, proxy credentials, private URLs, downloaded works, local state, or private API responses.
- [ ] I updated migration guidance for every breaking change.
