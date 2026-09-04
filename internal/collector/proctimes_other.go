//go:build !linux

package collector

import "github.com/shirou/gopsutil/v3/process"

// statReader has no state off Linux: there is no /proc to read, so each call
// goes through gopsutil. Kept as the same type so the caller does not need a
// build tag of its own.
type statReader struct{}

func newStatReader() *statReader { return &statReader{} }

// procStat returns a process's total CPU time and resident set size.
//
// The slower path, but the cheap one is Linux-specific and Linux is where this
// runs at scale.
func (r *statReader) procStat(pid int32) (cpuSecs float64, rss uint64, ok bool) {
	p, err := process.NewProcess(pid)
	if err != nil {
		return 0, 0, false
	}
	times, err := p.Times()
	if err != nil || times == nil {
		return 0, 0, false
	}
	// Memory is best-effort: a process without readable memory info still has
	// usable CPU numbers.
	if info, err := p.MemoryInfo(); err == nil && info != nil {
		rss = info.RSS
	}
	return times.User + times.System, rss, true
}
