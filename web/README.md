# SysSentient Web Dashboard

React 19 + Vite + Tailwind v4 frontend for the SysSentient daemon.

## Local development

```bash
npm install
npm run dev
```

The dev server runs on port `3000` and proxies `/api`, `/health` and `/ws` to
the daemon at `http://localhost:8080`.

## Build-time configuration

Only two variables exist, and both are ordinary connection settings:

```bash
VITE_SYS_SENTIENT_API_URL=http://localhost:8080/api
VITE_SYS_SENTIENT_WS_URL=ws://localhost:8080/ws/metrics
```

Both default to the page's own origin, so a normal deployment needs neither:
the daemon serves this bundle itself.

> **Never put a credential in a `VITE_` variable.** Vite inlines them into the
> published bundle at build time, so anyone who can load the dashboard can read
> them. A previous `VITE_SYS_SENTIENT_API_KEY` was removed for exactly this
> reason, along with the `?api_key=` WebSocket query parameter, which leaked the
> key into proxy and browser-history logs.
>
> Browser users sign in at `/login` and receive an `HttpOnly` session cookie.
> Machine clients send `X-API-Key`, which is configured server-side only.
> Provider secrets such as `SYS_SENTIENT_GEMINI_API_KEY` stay in the daemon's
> environment and never reach the browser.

## Design system

Colour, radius and type are CSS custom properties in `index.css`, mapped into
Tailwind through `@theme inline`. Tailwind v4 does **not** auto-load
`tailwind.config.js` — the `@config` directive at the top of `index.css` is what
loads it, and `styles.config.test.ts` plus `npm run verify:css` guard that.
Removing either lets the whole palette silently compile to nothing.

## Validation

```bash
npm audit --audit-level=moderate
npm run typecheck
npm test
npm run build      # includes verify:css
```
