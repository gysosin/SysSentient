## What and why

<!-- What changes, and what problem it solves. If it fixes a bug, what the
     symptom was. -->

## Verification

<!-- Paste real output, not "should work". -->

```
go vet ./... && go test ./... -race
cd web && npm run typecheck && npm test && npm run build
```

## Checklist

- [ ] `make verify` passes
- [ ] README / CHANGELOG / `docs/` updated in the same commit as the change
- [ ] New behaviour has a test, and I confirmed it fails without the change
- [ ] No credentials, databases or build output committed
