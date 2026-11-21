//go:build linux

package monitoring

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	pkg_flags "github.com/komari-monitor/komari-agent/cmd/flags"
)

// nicSpeedTracker keeps the previous network counter snapshot so that the
// transfer rate can be derived without sleeping between two samples.
type nicSpeedTrackerState struct {
	mu            sync.Mutex
	primed        bool
	up            uint64
	down          uint64
	lastUpSpeed   uint64
	lastDownSpeed uint64
	at            time.Time
}

var nicSpeedTracker = &nicSpeedTrackerState{}

// procNetBase returns the directory that holds the kernel connection tables.
// Inside a container the agent may be able to see the host /proc.
func procNetBase() string {
	hostProc := pkg_flags.GlobalConfig.HostProc
	if hostProc != "" {
		netDir := filepath.Join(hostProc, "net")
		if info, err := os.Stat(netDir); err == nil && info.IsDir() {
			return netDir
		}
	}
	return "/proc/net"
}

// countProcNetFile counts data rows in a /proc/net connection table.
// It streams over the file instead of materialising one struct per connection,
// which keeps allocations near zero even on hosts with tens of thousands of
// sockets.
func countProcNetFile(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	count := -1 // skip the header line
	buf := make([]byte, 32*1024)
	for {
		n, readErr := f.Read(buf)
		if n > 0 {
			count += bytes.Count(buf[:n], []byte{'\n'})
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			return count, readErr
		}
	}
	if count < 0 {
		count = 0
	}
	return count, nil
}

// connectionsFromProc counts TCP/UDP sockets straight from the kernel tables.
func connectionsFromProc() (tcpCount, udpCount int, err error) {
	base := procNetBase()

	tcp, tcpErr := countProcNetFile(filepath.Join(base, "tcp"))
	if tcpErr == nil {
		if tcp6, err6 := countProcNetFile(filepath.Join(base, "tcp6")); err6 == nil {
			tcp += tcp6
		}
		tcpCount = tcp
	}

	udp, udpErr := countProcNetFile(filepath.Join(base, "udp"))
	if udpErr == nil {
		if udp6, err6 := countProcNetFile(filepath.Join(base, "udp6")); err6 == nil {
			udp += udp6
		}
		udpCount = udp
	}

	if tcpErr != nil && udpErr != nil {
		return 0, 0, tcpErr
	}
	return tcpCount, udpCount, nil
}
