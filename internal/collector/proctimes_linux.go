//go:build linux

package collector

import (
	"os"
	"strconv"
)

// userHZ is the kernel's clock tick. Configurable at build time in principle,
// but 100 on every mainstream Linux distribution, and gopsutil assumes the
// same.
const userHZ = 100.0

// statReader reads /proc/<pid>/stat with a reusable buffer.
//
// This runs once per process per poll — roughly 600 times every two seconds,
// forever — so the allocations matter more than the code's tidiness. A scratch
// buffer and manual field scanning keep the whole scan close to allocation
// free; os.ReadFile plus strings.Fields allocated twice per process, which at
// this rate is most of a megabyte of garbage per collection.
type statReader struct {
	buf  []byte
	path []byte
}

func newStatReader() *statReader {
	return &statReader{
		buf:  make([]byte, 0, 512),
		path: make([]byte, 0, 32),
	}
}

// cpuSeconds returns a process's total CPU time (utime + stime).
//
// gopsutil's Times() answers the same question but builds a full CPUTimesStat,
// allocating roughly fifty objects each time. Measured across ~600 processes
// that was ~57ms and 13.5 MB of garbage per collection — the largest single
// cost in the collector.
//
// Only utime and stime are read. The other per-CPU accounting fields the
// previous code summed (nice, iowait, irq, softirq, steal) do not appear in a
// per-process stat line at all, so it was adding zeroes; that claim is checked
// against gopsutil in proctimes_test.go rather than assumed.
func (r *statReader) cpuSeconds(pid int32) (float64, bool) {
	r.path = append(r.path[:0], "/proc/"...)
	r.path = strconv.AppendInt(r.path, int64(pid), 10)
	r.path = append(r.path, "/stat"...)

	// #nosec G304 -- pid comes from the kernel's own process list.
	f, err := os.Open(string(r.path))
	if err != nil {
		return 0, false
	}
	defer func() { _ = f.Close() }()

	r.buf = r.buf[:cap(r.buf)]
	n, err := f.Read(r.buf)
	if err != nil || n == 0 {
		return 0, false
	}
	line := r.buf[:n]

	// Field 2 is the executable name in parentheses and may itself contain
	// spaces and parentheses — "(Web Content)", "(a) b)" — so splitting the
	// whole line on spaces is wrong. Everything after the *last* ')' is
	// well-behaved, space-separated fields.
	closeIdx := -1
	for i := len(line) - 1; i >= 0; i-- {
		if line[i] == ')' {
			closeIdx = i
			break
		}
	}
	if closeIdx < 0 || closeIdx+2 >= len(line) {
		return 0, false
	}

	// After the name, fields restart at 3 (state). utime is field 14 and stime
	// 15, so the 12th and 13th tokens of this remainder.
	const (
		utimeField = 12
		stimeField = 13
	)
	var utime, stime float64
	field, start := 1, closeIdx+2
	for i := start; i <= len(line); i++ {
		if i < len(line) && line[i] != ' ' {
			continue
		}
		if i > start {
			switch field {
			case utimeField:
				utime = parseUint(line[start:i])
			case stimeField:
				stime = parseUint(line[start:i])
			}
			field++
			if field > stimeField {
				break
			}
		}
		start = i + 1
	}
	if field <= stimeField {
		return 0, false
	}

	return (utime + stime) / userHZ, true
}

// parseUint avoids strconv.ParseFloat's generality — these are always small
// non-negative integers — and takes a byte slice so no string is allocated.
func parseUint(b []byte) float64 {
	var n float64
	for _, c := range b {
		if c < '0' || c > '9' {
			return n
		}
		n = n*10 + float64(c-'0')
	}
	return n
}
