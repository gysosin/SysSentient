package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"golang.org/x/term"
)

// announceFirstRun makes the setup token impossible to miss.
//
// It was logged as one structured line among a dozen others at startup. That
// is fine for an operator tailing a journal and useless for someone who
// double-clicked a binary: the single thing they must do is buried in output
// that looks like an error report.
func announceFirstRun(w io.Writer, url, token string) {
	// Only decorate a real terminal. In a journal or a container log the box
	// drawing is noise, and the structured log line already carries the token.
	if f, ok := w.(*os.File); !ok || !term.IsTerminal(int(f.Fd())) {
		return
	}
	// A write to the operator's terminal; nothing useful to do if it fails.
	_, _ = fmt.Fprint(w, renderFirstRun(url, token))
}

// renderFirstRun formats the notice.
//
// Separated from the terminal check so the formatting is testable: a test
// cannot hand announceFirstRun a real terminal, and a test that therefore
// asserts nothing is worse than no test.
func renderFirstRun(url, token string) string {
	line := strings.Repeat("─", 68)
	var b strings.Builder
	fmt.Fprintf(&b, "\n┌%s┐\n", line)
	for _, row := range []string{
		"SysSentient is running. One step left.",
		"",
		"Open:  " + url,
		"Token: " + token,
		"",
		"The token is shown once and creates the first administrator.",
	} {
		fmt.Fprintf(&b, "│ %-66s │\n", row)
	}
	fmt.Fprintf(&b, "└%s┘\n\n", line)
	return b.String()
}

// openBrowser tries to show the setup page.
//
// Best effort by design: a headless server has no browser, and failing to open
// one is not a reason to refuse to start. The token is on screen either way, so
// nothing is lost when this does nothing.
//
// The URL deliberately carries no token. A secret in a URL ends up in browser
// history, and the operator can paste it from the terminal.
func openBrowser(url string) {
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		// A service or a container: nobody is watching a desktop session.
		return
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		if _, err := exec.LookPath("xdg-open"); err != nil {
			return
		}
		cmd = exec.Command("xdg-open", url)
	}

	// Detached and ignored: the browser outlives this call, and its exit code
	// tells us nothing about whether a page opened.
	_ = cmd.Start()
}
