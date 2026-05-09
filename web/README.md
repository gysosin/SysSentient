# SysSentient Web Dashboard

React/Vite frontend for the SysSentient daemon.

## Local Development

```bash
npm install
npm run dev
```

The dev server runs on port `3000` and talks to the daemon API at
`http://localhost:8080` by default.

## Build-Time Configuration

Only expose dashboard connection settings with `VITE_` variables:

```bash
VITE_SYS_SENTIENT_API_URL=http://localhost:8080/api
VITE_SYS_SENTIENT_WS_URL=ws://localhost:8080/ws/metrics
VITE_SYS_SENTIENT_API_KEY=your-dashboard-key
```

Provider secrets such as `SYS_SENTIENT_GEMINI_API_KEY` must stay server-side in
the Go daemon environment.

## Validation

```bash
npm audit --audit-level=moderate
npm test
npm run typecheck
npm run build
```
