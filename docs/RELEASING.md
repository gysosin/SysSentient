# Releasing

## Cutting a release

1. Make sure `main` is green.
2. Move `CHANGELOG.md`'s `[Unreleased]` section under a new version heading
   with today's date, and add the comparison links at the bottom.
3. Tag and push:

```bash
git tag -a v0.2.0 -m "SysSentient 0.2.0"
git push origin v0.2.0
```

The tag triggers `.github/workflows/release.yml`, which builds everything with
GoReleaser and publishes the release.

## What a tag produces

| | |
|---|---|
| Archives | linux/darwin `.tar.gz`, windows `.zip` — amd64 and arm64 |
| Packages | `.deb`, `.rpm`, `.apk` — amd64 and arm64 |
| Container | multi-arch image on GHCR |
| Plus | `checksums.txt` and generated release notes |

## Testing the pipeline without tagging

```bash
goreleaser check
goreleaser release --snapshot --clean
ls dist/
```

The snapshot build runs everything except publishing, so a configuration
mistake is found before a tag exists rather than after.

## Version stamping

Version, commit and build date are injected with `-ldflags -X` into
`internal/version`. They surface in `--version`, the start-up log line,
`GET /health` and the dashboard footer.

`make daemon` runs `./sys-sentient --version` as a self-check, so a broken stamp
fails the build rather than shipping.
