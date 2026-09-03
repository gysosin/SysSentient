package agent

import "os"

// osWriteFile is a thin indirection so tests can corrupt a spool file.
func osWriteFile(path string, data []byte) error {
	return os.WriteFile(path, data, 0o600)
}
