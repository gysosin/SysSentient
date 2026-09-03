# 13 — Readable type and a full-width layout

| | |
|---|---|
| **Phase** | 4 — Console |
| **Depends on** | 00 |
| **Status** | **done** — 12px floor, fluid layout, guarded by a test |

## Why

**72 hardcoded font sizes below 14px**: 4×9px, 27×10px, 23×11px, 12×12px,
6×13px. Log timestamps sit on `text-melt`, the dimmest step in the ramp — which
is exactly the "the time is very small" complaint.

`max-w-[1600px]` appears in 7 places in `AppShell.tsx`; on a 2560px monitor that
is ~960px of dead gutter.

Note `tailwind.config.js` once carried a comment calling 15 such usages "below
any legibility floor". This regressed, and largely during the redesign port —
hence the guard below.

## Scope

- Define a real type scale in `web/index.css` and replace every `text-[Npx]`.
- Floor: 13px body, 12px labels. Lift timestamps off `text-melt`.
- Replace `max-w-[1600px]` with a fluid container; keep a readable cap on prose
  only.
- Rework nav-bar density; give the host switcher and feed state real prominence.

## Acceptance criteria

- Zero `text-[Npx]` below 12px remain.
- A test asserts this, so it cannot regress a third time.
- The layout uses the full viewport at 2560px with no dead gutter.
- Contrast passes WCAG AA at the new sizes.

## Verification

```bash
grep -rc 'text-\[\(9\|10\|11\)px\]' web/components web/pages   # expect 0
cd web && npm test
```

---

Every shard must also pass the project gate before it is pushed:

```bash
GOTOOLCHAIN=auto go vet ./... && GOTOOLCHAIN=auto go test ./... -race
cd web && npm audit --audit-level=moderate && npm run typecheck && npm test && npm run build
```
