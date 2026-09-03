# SysSentient - Quick Start Guide

## 🚀 First Time Setup

### 1. Install Dependencies
```bash
# Go 1.25.10+ or GOTOOLCHAIN=auto
go version

# Node.js 22+ (for web UI)
node --version
npm --version
```

### 2. Build the Project
```bash
# Build web UI
cd web
npm install
npm run build
cd ..

# Build daemon
go build -o sys-daemon ./cmd/daemon
```

### 3. Configure the AI (optional)
```bash
# Without this the daemon runs fine; AI analysis is simply disabled.
export SYS_SENTIENT_GEMINI_API_KEY="your-gemini-api-key"
```

### 4. Run the Daemon
```bash
./sys-daemon
```

### 5. Create the first admin
On first start the daemon has no accounts, so it prints a one-time setup token:

```
level=WARN msg="FIRST-RUN SETUP REQUIRED: no users exist yet"
      url=http://localhost:8080/setup token=<43-character token>
```

Open that URL, paste the token, and choose an email and a password of at least
12 characters. There is no default password. Afterwards sign in at
`http://localhost:8080/login`; add more users under **Settings → Users**.

Scripts authenticate separately with `SYS_SENTIENT_SERVER_API_KEY`, sent as an
`X-API-Key` header. Do not commit real keys to source.

---

## 🔑 Authentication Setup

### Generate Secure API Key
```bash
# Linux/Mac
openssl rand -base64 32

# Or use any strong random string
```

### Set in Environment
```bash
export SYS_SENTIENT_SERVER_API_KEY="your-generated-key"
```

### Frontend auth
Nothing to configure. The dashboard is served by the daemon itself, so it calls
the API same-origin and the browser attaches the session cookie automatically.

For local Vite development (`npm run dev` on port 3000), `vite.config.ts`
proxies `/api`, `/health` and `/ws` to the daemon on port 8080, so the same
cookie works there too.

The dashboard bundle no longer contains any credential. The earlier
`VITE_SYS_SENTIENT_API_KEY` build variable has been removed: Vite inlined it
into the published JavaScript, where anyone who could load the page could read
it.

---

## 🧪 Testing

### Run All Tests
```bash
GOTOOLCHAIN=auto go vet ./...
GOTOOLCHAIN=auto go test ./... -v
cd web && npm test
```

### Check Build
```bash
GOTOOLCHAIN=auto go build ./cmd/daemon
cd web && npm run typecheck && npm run build
```

### Security Checks
```bash
GOTOOLCHAIN=auto go run golang.org/x/vuln/cmd/govulncheck@latest ./...
cd web && npm audit --audit-level=moderate
```

---

## 📋 Common Tasks

### Check Service Health
```bash
curl http://localhost:8080/health
```

### Fetch Latest Metrics (with auth)
```bash
curl -H "X-API-Key: your-key" http://localhost:8080/api/metrics
```

### Trigger Manual AI Analysis
```bash
curl -X POST -H "X-API-Key: your-key" http://localhost:8080/api/analyze
```

### View Logs (if daemon doesn't have permissions)
```bash
# Grant dmesg access (one time)
sudo chmod +r /dev/kmsg

# Or run daemon with sudo (not recommended for production)
sudo ./sys-daemon
```

---

## ⚙️ Configuration

### Option 1: Environment Variables
```bash
export SYS_SENTIENT_SERVER_PORT=8080
export SYS_SENTIENT_SERVER_API_KEY="your-key"
export SYS_SENTIENT_GEMINI_API_KEY="gemini-key"
export SYS_SENTIENT_COLLECTOR_POLL_INTERVAL_SECONDS=2
```

### Option 2: config.yaml
Create `config.yaml` in project root:

```yaml
server:
  port: 8080
  api_key: "your-secure-key"
  allowed_origins:
    - "http://localhost:8080"
    - "http://localhost:5173"

gemini:
  api_key: "your-gemini-api-key"
  model_name: "gemini-2.5-flash-lite"
  max_daily_cost: 1.0

collector:
  poll_interval_seconds: 2

database:
  path: "sys-sentient.db"
  metrics_retention_hours: 24
  insights_retention_hours: 168
```

---

## 🐛 Troubleshooting

### "Failed to collect logs"
**Cause**: Insufficient permissions for journalctl/dmesg
**Fix**:
```bash
# Add user to systemd-journal group
sudo usermod -aG systemd-journal $USER
newgrp systemd-journal

# Or run with sudo (testing only)
sudo ./sys-daemon
```

### "AI Service unavailable (circuit breaker open)"
**Cause**: Multiple Gemini API failures
**Fix**:
- Check your API key
- Wait 2 minutes for automatic reset
- Check internet connectivity

### "Unauthorized" errors
**Cause**: API key mismatch or missing
**Fix**:
- Verify `SYS_SENTIENT_SERVER_API_KEY` is set
- Ensure frontend sends `X-API-Key` header
- Or disable auth by unsetting the API key

### Database errors
**Cause**: File permissions or disk space
**Fix**:
```bash
# Check permissions
ls -l sys-sentient.db*

# Check disk space
df -h .
```

---

## 🔒 Security Best Practices

1. **Always use HTTPS in production** (use nginx/Caddy reverse proxy)
2. **Use strong API keys** (32+ characters, random)
3. **Restrict CORS origins** (never use `["*"]` in production)
4. **Run as non-root user** (create dedicated `syssentinel` user)
5. **Limit file permissions**:
   ```bash
   chmod 600 config.yaml
   chmod 644 sys-sentient.db
   ```

---

## 📊 Performance Tips

1. **Adjust poll interval** for your workload:
   - High-frequency monitoring: `poll_interval_seconds: 1`
   - Normal usage: `poll_interval_seconds: 2` (default)
   - Low resource usage: `poll_interval_seconds: 5`

2. **Database maintenance**:
   - Old metrics auto-prune after 24 hours
   - Manual cleanup: `DELETE FROM metrics WHERE timestamp < datetime('now', '-7 days')`

3. **Reduce AI costs**:
   - RAG cache automatically deduplicates similar analyses
   - Circuit breaker prevents wasted calls during outages
   - Adjust `max_daily_cost` to set budget limit

---

## 🎯 Next Steps

1. ✅ Review [CHANGELOG.md](CHANGELOG.md) for recent changes
2. ✅ Configure authentication for security
3. ✅ Set up systemd service for auto-start
4. ✅ Add to monitoring/alerting system
5. ✅ Configure HTTPS reverse proxy

---

## 💡 Tips

- **Development Mode**: Disable auth by not setting `SERVER_API_KEY`
- **Debug Mode**: Watch logs with `./sys-daemon 2>&1 | tee daemon.log`
- **Frontend Dev**: `cd web && npm run dev` for hot reload
- **Test WebSocket**: Use `websocat` or browser console

---

## 📚 Additional Resources

- Main README: [README.md](README.md)
- Changelog: [CHANGELOG.md](CHANGELOG.md)
- Security notes: [SECURITY.md](SECURITY.md)
- Contributing: [CONTRIBUTING.md](CONTRIBUTING.md)
- Architecture Review: [architecture_review.md](architecture_review.md)
- Project Plan: [plan.md](plan.md)

---

**Need Help?** Check existing issues or create a new one with:
- System info (`uname -a`, `go version`)
- Error messages
- Configuration (redact API keys!)
