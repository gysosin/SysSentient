// Package installer holds the scripts a new machine uses to install itself.
//
// They live beside their embed rather than in a top-level scripts/ directory
// because Go can only embed files at or below the embedding package, and two
// copies of a script that must stay identical is a trap: the one you edit is
// never the one that ships.
package installer

import _ "embed"

// Shell is the POSIX installer, served at /install.sh.
//
//go:embed install.sh
var Shell string

// PowerShell is the Windows installer, served at /install.ps1.
//
//go:embed install.ps1
var PowerShell string
