# Third-party components

SysSentient is licensed under [Apache-2.0](../LICENSE). The components below
are redistributed with it, each under its own licence.

Nothing here is copyleft, so the Apache-2.0 choice is unencumbered.

## Go modules

Direct dependencies. Licences were read from each module in the module cache,
not from memory.

| Module | Licence | Used for |
|---|---|---|
| `github.com/gorilla/websocket` | BSD-2-Clause | the live metric stream |
| `github.com/shirou/gopsutil/v3` | BSD-3-Clause | cross-platform metric collection |
| `github.com/spf13/viper` | MIT | configuration loading |
| `golang.org/x/crypto` | BSD-3-Clause | argon2id password hashing |
| `golang.org/x/sys` | BSD-3-Clause | Windows registry access for machine identity |
| `google.golang.org/genai` | Apache-2.0 | Gemini analysis |
| `modernc.org/sqlite` | BSD-3-Clause | storage; pure Go, which is what makes a static binary possible |

Regenerate the full transitive list with:

```bash
go run github.com/google/go-licenses@latest report ./...
```

## Frontend

| Package | Licence |
|---|---|
| React, React DOM | MIT |
| react-router | MIT |
| Radix UI primitives | MIT |
| Tailwind CSS | MIT |
| recharts | MIT |
| motion | MIT |
| lucide-react | ISC |
| class-variance-authority, clsx, tailwind-merge | MIT |

## Bundled typefaces

**These ship as binaries inside `sys-sentient`**, embedded through
`web/embed.go`, so their terms travel with every copy of the product.

| Typeface | Licence | Copyright |
|---|---|---|
| [Sora](https://github.com/jonathansoma/sora) | SIL Open Font License 1.1 | Copyright 2020 The Sora Project Authors |
| [Manrope](https://github.com/sharanda/manrope) | SIL Open Font License 1.1 | Copyright 2018 The Manrope Project Authors |
| [JetBrains Mono](https://github.com/JetBrains/JetBrainsMono) | SIL Open Font License 1.1 | Copyright 2020 The JetBrains Mono Project Authors |

The OFL requires that this notice accompany the font files. It permits
redistribution, including bundled inside another product, and does not extend
its terms to the rest of the software.

Full licence texts are in each package under
`web/node_modules/@fontsource-variable/*/LICENSE`.

## Why the fonts are bundled rather than fetched

SysSentient is deployed on firewalled and air-gapped hosts where a font CDN is
unreachable. A remote font link does not fail loudly — it silently falls back to
a system face — so the typography would quietly change on exactly the
deployments this product is built for. Bundling makes the design a guarantee,
at the cost of carrying these licence obligations.
