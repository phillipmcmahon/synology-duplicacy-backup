package workflow

import (
	"context"
	"os"
	"os/signal"
	"strconv"
	"sync"
)

type restoreInterruptInfo struct {
	Signal      string
	Workspace   string
	Completed   int
	Total       int
	CurrentPath string
}

type restoreInterruptTracker struct {
	mu      sync.Mutex
	info    restoreInterruptInfo
	started bool
}

func newRestoreInterruptContext(rt Runtime, progress RestoreProgress, workspace string) (context.Context, func(), *restoreInterruptTracker) {
	ctx, cancel := context.WithCancel(context.Background())
	tracker := &restoreInterruptTracker{
		info: restoreInterruptInfo{Workspace: workspace},
	}
	sigChan := make(chan os.Signal, 1)
	if rt.SignalNotify != nil {
		rt.SignalNotify(sigChan, SignalSet()...)
	}

	done := make(chan struct{})
	var once sync.Once
	go func() {
		select {
		case sig := <-sigChan:
			info := tracker.markInterrupted(sig)
			progress.PrintInterrupted(info)
			cancel()
		case <-done:
		}
	}()

	cleanup := func() {
		once.Do(func() {
			close(done)
			signal.Stop(sigChan)
			cancel()
		})
	}
	return ctx, cleanup, tracker
}

func (t *restoreInterruptTracker) setCurrent(current, total int, path string) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.started = true
	t.info.Total = total
	t.info.CurrentPath = restoreProgressPath(path)
	if current > 0 {
		t.info.Completed = current - 1
	}
}

func (t *restoreInterruptTracker) setCompleted(completed, total int) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.started = true
	t.info.Completed = completed
	t.info.Total = total
}

func (t *restoreInterruptTracker) markInterrupted(sig os.Signal) restoreInterruptInfo {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.info.Signal = sig.String()
	if !t.started && t.info.Total == 0 {
		t.info.Total = 1
	}
	return t.info
}

func restoreInterruptProgress(info restoreInterruptInfo) string {
	if info.Total <= 0 {
		return "unknown"
	}
	return strconv.Itoa(info.Completed) + " of " + strconv.Itoa(info.Total)
}
