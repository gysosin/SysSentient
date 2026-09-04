package collector

import (
	"math"
	"os"
	"testing"

	"github.com/shirou/gopsutil/v3/process"
)

// procCPUSeconds replaced gopsutil's Times() for speed. Speed is worthless if
// the number is wrong, so this compares the two on real processes.
//
// It also checks the claim made in the comment there: that the extra CPU
// fields the previous code summed (Nice, Iowait, Irq, Softirq, Steal) are not
// populated per-process on Linux, so dropping them changes nothing.
func TestProcCPUSecondsMatchesGopsutil(t *testing.T) {
	pids, err := process.Pids()
	if err != nil {
		t.Skipf("cannot list pids: %v", err)
	}

	compared, extras := 0, 0
	for _, pid := range pids {
		p, err := process.NewProcess(pid)
		if err != nil {
			continue
		}
		times, err := p.Times()
		if err != nil || times == nil {
			continue
		}
		mine, _, ok := newStatReader().procStat(pid)
		if !ok {
			// A process that exited between the two reads is expected.
			continue
		}

		want := times.User + times.System
		// Both sides read a live counter that advances between the two calls,
		// so an exact match is not available. One clock tick of tolerance.
		if math.Abs(mine-want) > 0.02 {
			t.Errorf("pid %d: procCPUSeconds=%.4f, gopsutil User+System=%.4f", pid, mine, want)
		}
		if times.Nice+times.Iowait+times.Irq+times.Softirq+times.Steal != 0 {
			extras++
		}
		compared++
		if compared >= 40 {
			break
		}
	}

	if compared == 0 {
		t.Skip("no comparable processes")
	}
	if extras > 0 {
		t.Errorf("%d of %d processes reported non-zero Nice/Iowait/Irq/Softirq/Steal; "+
			"dropping them from the sum is not safe after all", extras, compared)
	}
	t.Logf("compared %d processes, all within one clock tick", compared)
}

// A process name can contain spaces and parentheses, which is why the parser
// keys off the last ')' rather than splitting the whole line.
func TestProcCPUSecondsHandlesOddProcessNames(t *testing.T) {
	if _, err := os.Stat("/proc/self/stat"); err != nil {
		t.Skip("no /proc")
	}
	// Self is guaranteed to exist and to be readable.
	if _, _, ok := newStatReader().procStat(int32(os.Getpid())); !ok {
		t.Fatal("could not read own /proc stat")
	}
}
