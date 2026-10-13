package server

import (
	"net/http"
	"sync"
	"time"
)

// lifeline ends the program when its window goes away. The page keeps one
// request to /api/alive open for as long as it lives; closing the window,
// a crashed browser or a navigation all drop that connection. A reload
// reconnects within moments, so the program waits a little before quitting.
type lifeline struct {
	mu          sync.Mutex
	started     time.Time
	open        int       // connected pages
	everOpened  bool      // a page has connected at least once
	lastDropped time.Time // when the last page disconnected
	closed      chan struct{}
}

const (
	firstConnectWait = 2 * time.Minute // the window may take a while to appear
	reconnectGrace   = 5 * time.Second // long enough for a reload
)

func newLifeline() *lifeline {
	return &lifeline{started: time.Now(), closed: make(chan struct{})}
}

// serve holds the page's connection open until the page goes away.
func (l *lifeline) serve(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(http.StatusOK)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}

	l.mu.Lock()
	l.open++
	l.everOpened = true
	l.mu.Unlock()

	<-r.Context().Done()

	l.mu.Lock()
	l.open--
	l.lastDropped = time.Now()
	l.mu.Unlock()
}

func (l *lifeline) watch() {
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for now := range tick.C {
		if l.expired(now) {
			close(l.closed)
			return
		}
	}
}

func (l *lifeline) expired(now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	switch {
	case l.open > 0:
		return false
	case !l.everOpened:
		return now.Sub(l.started) > firstConnectWait
	default:
		return now.Sub(l.lastDropped) > reconnectGrace
	}
}
