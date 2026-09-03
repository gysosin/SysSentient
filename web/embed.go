// Package web embeds the built dashboard into the daemon binary.
//
// Before this, the server read the dashboard from "./web/dist" relative to the
// process working directory. That made the binary non-relocatable — it only
// worked when started from the repository root or from /opt/sys-sentient,
// which is why the shipped systemd unit had to pin WorkingDirectory. No .deb,
// .rpm, .msi or tarball can ship a binary with that constraint, so embedding
// is a prerequisite for packaging at all.
package web

import (
	"embed"
	"io/fs"
)

// dist holds the compiled dashboard. `all:` is required so Vite's hashed
// assets and any dotfiles are included; the default pattern skips names
// beginning with "." or "_".
//
//go:embed all:dist
var dist embed.FS

// Dist returns the embedded dashboard rooted at the directory that contains
// index.html, or ok=false when the binary was built without a real dashboard.
//
// A build with no `npm run build` still compiles — dist/.gitkeep satisfies the
// embed pattern — so the emptiness has to be detected at runtime rather than
// by the compiler. Callers fall back to serving from disk and say so, which is
// far friendlier than a binary that serves a blank page.
func Dist() (fs.FS, bool) {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		return nil, false
	}
	if _, err := fs.Stat(sub, "index.html"); err != nil {
		return nil, false
	}
	return sub, true
}
