package core

import (
	"sync"
	"time"
)

type timerTask struct {
	key      string
	deadline time.Time
	fn       func()
}

type TimerWheel struct {
	mu       sync.Mutex
	interval time.Duration
	stopCh   chan struct{}
	tasks    map[string]timerTask
}

func NewTimerWheel(interval time.Duration) *TimerWheel {
	return &TimerWheel{
		interval: interval,
		stopCh:   make(chan struct{}),
		tasks:    make(map[string]timerTask),
	}
}

func (tw *TimerWheel) Schedule(key string, delay time.Duration, fn func()) {
	tw.mu.Lock()
	defer tw.mu.Unlock()
	tw.tasks[key] = timerTask{key: key, deadline: time.Now().Add(delay), fn: fn}
}

func (tw *TimerWheel) Cancel(key string) {
	tw.mu.Lock()
	defer tw.mu.Unlock()
	delete(tw.tasks, key)
}

func (tw *TimerWheel) Run() {
	ticker := time.NewTicker(tw.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			tw.fireExpired()
		case <-tw.stopCh:
			return
		}
	}
}

func (tw *TimerWheel) Stop() {
	close(tw.stopCh)
}

func (tw *TimerWheel) fireExpired() {
	now := time.Now()
	var due []func()
	tw.mu.Lock()
	for k, task := range tw.tasks {
		if !task.deadline.After(now) {
			due = append(due, task.fn)
			delete(tw.tasks, k)
		}
	}
	tw.mu.Unlock()
	for _, fn := range due {
		fn()
	}
}
