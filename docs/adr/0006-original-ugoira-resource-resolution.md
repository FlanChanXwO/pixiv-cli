# Resolve original ugoira resources through web metadata

When `quality=original` is requested, authenticated sessions may query Pixiv web metadata solely to resolve the original ugoira zip because the App API metadata exposes only the medium variant. This is deliberate resource-version selection, not an automatic fallback after an App API error; all other source-routing rules remain unchanged.
