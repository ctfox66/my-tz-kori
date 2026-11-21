//go:build !linux

package monitoring

import (
	"errors"
	"sync"
	"time"
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

// connectionsFromProc is only implemented for Linux. Other platforms fall back
// to gopsutil enumeration in ConnectionsCount.
func connectionsFromProc() (tcpCount, udpCount int, err error) {
	return 0, 0, errors.New("proc net tables unavailable on this platform")
}
