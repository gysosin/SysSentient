# SysSentient

Self-hosted server monitoring with alerting and AI-assisted diagnosis. One
static binary, 0.78% idle CPU, no telemetry.

- **Website** — https://gysosin.github.io/SysSentient/
- **Install** — [docs/INSTALL.md](https://github.com/gysosin/SysSentient/blob/main/docs/INSTALL.md)
- **Latest release** — [v0.1.0-beta.1](https://github.com/gysosin/SysSentient/releases/latest)

This wiki holds answers to questions that come up in use. The reference
documentation lives in the repository, where it is reviewed with the code that
it describes:

| Topic | Page |
|---|---|
| Install and first run | [INSTALL.md](https://github.com/gysosin/SysSentient/blob/main/docs/INSTALL.md) |
| Every configuration key | [CONFIGURATION.md](https://github.com/gysosin/SysSentient/blob/main/docs/CONFIGURATION.md) |
| Running a fleet | [DEPLOYMENT.md](https://github.com/gysosin/SysSentient/blob/main/docs/DEPLOYMENT.md) |
| What it costs to run | [PERFORMANCE.md](https://github.com/gysosin/SysSentient/blob/main/docs/PERFORMANCE.md) |
| What leaves the machine | [PRIVACY.md](https://github.com/gysosin/SysSentient/blob/main/docs/PRIVACY.md) |
| How it fits together | [ARCHITECTURE.md](https://github.com/gysosin/SysSentient/blob/main/docs/ARCHITECTURE.md) |

## Start here

```bash
sys-sentient
```

The dashboard is on `:8080`. On first run the daemon logs a one-time setup
token; open `/setup` and use it to create the admin account.

## Pages

- [[FAQ]]
- [[Troubleshooting]]
- [[Adding a machine]]
