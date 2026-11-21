package monitoring

import (
	"sync"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
)

// cpuUsageTracker derives CPU usage from the delta of two consecutive
// /proc/stat snapshots instead of blocking for a full second inside
// cpu.Percent(1s). A single call now costs microseconds.
type cpuUsageTracker struct {
	mu        sync.Mutex
	primed    bool
	lastTotal float64
	lastIdle  float64
	lastAt    time.Time
	lastUsage float64
}

var cpuTracker = &cpuUsageTracker{}

// CPUUsage returns the CPU usage percentage observed since the previous call.
// The first call only records a baseline and returns the last cached value
// (0 at startup) so callers never block.
func CPUUsage() float64 {
	cpuTracker.mu.Lock()
	defer cpuTracker.mu.Unlock()

	times, err := cpu.Times(false)
	if err != nil || len(times) == 0 {
		return cpuTracker.lastUsage
	}

	t := times[0]
	all := t.User + t.System + t.Idle + t.Nice + t.Iowait + t.Irq + t.Softirq + t.Steal + t.Guest + t.GuestNice
	idle := t.Idle + t.Iowait
	now := time.Now()

	prevTotal, prevIdle, prevAt := cpuTracker.lastTotal, cpuTracker.lastIdle, cpuTracker.lastAt
	primed := cpuTracker.primed

	cpuTracker.lastTotal = all
	cpuTracker.lastIdle = idle
	cpuTracker.lastAt = now
	cpuTracker.primed = true

	if !primed || prevAt.IsZero() || all <= prevTotal {
		return cpuTracker.lastUsage
	}
	// Counter reset or a very long gap: treat as a fresh baseline.
	if now.Sub(prevAt) > 10*time.Minute {
		return cpuTracker.lastUsage
	}

	totalDelta := all - prevTotal
	idleDelta := idle - prevIdle
	if idleDelta < 0 {
		idleDelta = 0
	}
	if idleDelta > totalDelta {
		idleDelta = totalDelta
	}
	if totalDelta <= 0 {
		return cpuTracker.lastUsage
	}

	usage := (1 - idleDelta/totalDelta) * 100
	if usage < 0 {
		usage = 0
	}
	if usage > 100 {
		usage = 100
	}
	cpuTracker.lastUsage = usage
	return usage
}
