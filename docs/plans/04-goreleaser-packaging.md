# 04 — Cross-platform packages and a release pipeline

| | |
|---|---|
| **Phase** | 1 — Packaging |
| **Depends on** | 01, 02, 03 |
| **Status** | **done** — 12 artifacts, verified end to end |

## Why

Today `release.yml` produces exactly one artifact — a hand-rolled
`linux_amd64` tarball — and the GHCR image has no `platforms:` key, so it is
single-arch too. CI runs every job on `ubuntu-latest`, so Windows and macOS
have never been compiled, let alone tested.

## Scope

- Adopt GoReleaser: linux/windows/darwin × amd64/arm64 static binaries with
  checksums.
- Packages via nfpm: `.deb`, `.rpm`, `.apk`. Homebrew tap. Scoop manifest.
- Windows service via `golang.org/x/sys/windows/svc`; macOS launchd plist;
  keep the systemd unit for Linux.
- Multi-arch GHCR image with OCI labels.
- Add `windows-latest` and `macos-latest` jobs to CI.
- Fix the `Dockerfile`'s missing `BuildDate` ldflag, and the dead relative
  `CHANGELOG.md` link in the release body.

## Acceptance criteria

- A tag produces binaries and packages for all six OS/arch combinations.
- `docker manifest inspect` shows both amd64 and arm64.
- CI compiles on Linux, Windows and macOS.

## Verification

```bash
goreleaser release --snapshot --clean
ls dist/
```
Then install the `.deb` in a clean container and the `.msi`/zip on a Windows VM.

---

Every shard must also pass the project gate before it is pushed:

```bash
GOTOOLCHAIN=auto go vet ./... && GOTOOLCHAIN=auto go test ./... -race
cd web && npm audit --audit-level=moderate && npm run typecheck && npm test && npm run build
```
