package core

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestTimerWheelFires(t *testing.T) {
	tw := NewTimerWheel(5 * time.Millisecond)
	defer tw.Stop()
	var fired atomic.Bool
	go tw.Run()
	tw.Schedule("x", 10*time.Millisecond, func() { fired.Store(true) })
	time.Sleep(40 * time.Millisecond)
	if !fired.Load() {
		t.Fatal("expected timer to fire")
	}
}
