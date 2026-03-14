package core

import (
	"sync"
	"sync/atomic"
	"time"

	"slpp/pkg/api"
)

type Stats struct {
	startedAt time.Time

	activeSessions    atomic.Int64
	activeChannels    atomic.Int64
	framesIn          atomic.Uint64
	framesOut         atomic.Uint64
	bytesIn           atomic.Uint64
	bytesOut          atomic.Uint64
	droppedUDP        atomic.Uint64
	expiredUDP        atomic.Uint64
	authFailures      atomic.Uint64
	reconnects        atomic.Uint64
	sessionQueueBytes atomic.Uint64
	channelQueueBytes atomic.Uint64
}

func NewStats() *Stats {
	return &Stats{startedAt: time.Now()}
}

func (s *Stats) AddActiveSession(delta int64) {
	s.activeSessions.Add(delta)
}

func (s *Stats) AddActiveChannel(delta int64) {
	s.activeChannels.Add(delta)
}

func (s *Stats) AddAuthFailure() {
	s.authFailures.Add(1)
}

func (s *Stats) AddExpiredUDP() {
	s.expiredUDP.Add(1)
}

func (s *Stats) AddDroppedUDP() {
	s.droppedUDP.Add(1)
}

func (s *Stats) SetSessionQueueBytes(v uint64) {
	s.sessionQueueBytes.Store(v)
}

func (s *Stats) SetChannelQueueBytes(v uint64) {
	s.channelQueueBytes.Store(v)
}

func (s *Stats) Snapshot() api.StatsSnapshot {
	return api.StatsSnapshot{
		StartedAt:         s.startedAt,
		ActiveSessions:    int(s.activeSessions.Load()),
		ActiveChannels:    int(s.activeChannels.Load()),
		FramesIn:          s.framesIn.Load(),
		FramesOut:         s.framesOut.Load(),
		BytesIn:           s.bytesIn.Load(),
		BytesOut:          s.bytesOut.Load(),
		DroppedUDP:        s.droppedUDP.Load(),
		ExpiredUDP:        s.expiredUDP.Load(),
		AuthFailures:      s.authFailures.Load(),
		Reconnects:        s.reconnects.Load(),
		SessionQueueBytes: s.sessionQueueBytes.Load(),
		ChannelQueueBytes: s.channelQueueBytes.Load(),
	}
}

type SafeMap[K comparable, V any] struct {
	mu sync.RWMutex
	m  map[K]V
}

func NewSafeMap[K comparable, V any]() *SafeMap[K, V] {
	return &SafeMap[K, V]{m: make(map[K]V)}
}

func (m *SafeMap[K, V]) Load(k K) (V, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	v, ok := m.m[k]
	return v, ok
}

func (m *SafeMap[K, V]) Store(k K, v V) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.m[k] = v
}

func (m *SafeMap[K, V]) Delete(k K) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.m, k)
}

func (m *SafeMap[K, V]) Range(fn func(K, V) bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for k, v := range m.m {
		if !fn(k, v) {
			return
		}
	}
}
