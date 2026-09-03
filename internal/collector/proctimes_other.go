//go:build !linux

package collector

import "github.com/shirou/gopsutil/v3/process"

// statReader has no state off Linux: there is no /proc to read, so each call
// goes through gopsutil. Kept as the same type so the caller does not need a
// build tag of its own.
type statReader struct{}

func newStatReader() *statReader { return &statReader{} }

// cpuSeconds returns a process's total CPU time via gopsutil.
//
// The slower path, but the cheap one is Linux-specific and Linux is where this
// runs at scale.
func (r *statReader) cpuSeconds(pid int32) (float64, bool) {
	p, err := process.NewProcess(pid)
	if err != nil {
		return 0, false
	}
	times, err := p.Times()
	if err != nil || times == nil {
		return 0, false
	}
	return times.User + times.System, true
}
