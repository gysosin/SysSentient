package server

import (
	"net/http"
	"strings"

	"sys-sentient/internal/installer"
)

// installScripts are embedded so the daemon can hand a new machine the means
// to install itself. Without this the Devices screen told an operator to
// "install SysSentient on the machine" and offered nothing to do it with: you
// had to already have the binary to be told how to enrol it.
type installScript struct {
	body        string
	contentType string
}

var installScriptRoutes = map[string]installScript{
	"/install.sh":  {body: installer.Shell, contentType: "text/x-shellscript; charset=utf-8"},
	"/install.ps1": {body: installer.PowerShell, contentType: "text/plain; charset=utf-8"},
}

// handleInstallScript serves the installer for a platform.
//
// Deliberately unauthenticated, like the join endpoint: a machine that has not
// enrolled yet has no credential to present, and the script itself grants
// nothing — enrolling still needs a single-use token.
func (s *Server) handleInstallScript(w http.ResponseWriter, r *http.Request) {
	script, ok := installScriptRoutes[r.URL.Path]
	if !ok {
		http.NotFound(w, r)
		return
	}

	// text/plain rather than a download: the whole point is to be piped into a
	// shell, and a Content-Disposition would make a browser save it instead.
	w.Header().Set("Content-Type", script.contentType)
	w.Header().Set("Cache-Control", "no-store")
	// The script is executed by whoever fetches it, so it must not be framed
	// or sniffed into something else.
	w.Header().Set("X-Content-Type-Options", "nosniff")

	if _, err := strings.NewReader(script.body).WriteTo(w); err != nil {
		// The client hung up mid-transfer; nothing useful to add.
		return
	}
}
