# Installing SysSentient

Every release ships static binaries and packages for linux, windows and darwin
on amd64 and arm64. The dashboard is embedded in the binary, so there is
nothing else to copy and it runs from any directory.

Download from the [releases page](https://github.com/gysosin/SysSentient/releases)
and verify against `checksums.txt` before installing.

## Debian, Ubuntu

```bash
sudo dpkg -i sys-sentient_<version>_linux_amd64.deb
```

## Fedora, RHEL, openSUSE

```bash
sudo rpm -i sys-sentient_<version>_linux_amd64.rpm
```

## Alpine

```bash
sudo apk add --allow-untrusted sys-sentient_<version>_linux_amd64.apk
```

The packages create the `sys-sentient` service account, install the systemd
unit, and place a configuration at `/etc/sys-sentient/config.yaml` marked
`noreplace`, so an upgrade never overwrites your edits.

They deliberately do **not** start the service. The first run mints a one-time
token for creating the first administrator, and a service started silently at
install time would emit that token into the journal before anyone was watching.

```bash
sudo systemctl enable --now sys-sentient
sudo journalctl -u sys-sentient | grep -i "setup token"
```

Then open `http://localhost:8080` and create the first administrator. There is
no default password at any point.

## Windows and macOS

Download the archive, extract it, and run `sys-sentient`. Metric collection works
on both; see [ARCHITECTURE.md](ARCHITECTURE.md#platform-support) for what
differs.

A Windows service wrapper and a launchd plist are tracked in
[docs/plans/04-goreleaser-packaging.md](plans/04-goreleaser-packaging.md).

## Container

```bash
docker run --rm -p 127.0.0.1:8080:8080 \
  -v sys-sentient-data:/var/lib/sys-sentient \
  ghcr.io/gysosin/syssentient:latest
```

Images are multi-arch (amd64 and arm64).

## From source

```bash
make build      # dashboard, then daemon
./sys-sentient
```

`make help` lists every target. The dashboard must be built before the daemon,
because it is embedded at compile time.

## Before exposing it

The daemon serves plain HTTP. Terminate TLS in front of it and read
[SECURITY.md](../SECURITY.md) before letting anything but localhost reach it.
