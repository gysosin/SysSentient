# Quick start

This guide has been split into focused pages under [`docs/`](docs/README.md),
so each topic has exactly one home rather than three that drift apart.

| You want to | Read |
|---|---|
| Install it | [docs/INSTALL.md](docs/INSTALL.md) |
| Configure it | [docs/CONFIGURATION.md](docs/CONFIGURATION.md) |
| Deploy agents to a fleet | [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md) |
| Understand what it costs | [docs/PERFORMANCE.md](docs/PERFORMANCE.md) |
| Know what leaves the machine | [docs/PRIVACY.md](docs/PRIVACY.md) |
| Work on it | [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md) |
| Report a vulnerability | [SECURITY.md](SECURITY.md) |

## The short version

```bash
make build      # dashboard, then daemon
./sys-sentient    # dashboard on http://localhost:8080
```

On first run the daemon logs a one-time setup token. Use it at `/setup` to
create the first administrator; there is no default password at any point.

Without a Gemini API key the daemon runs with AI analysis disabled and nothing
leaves the machine.
