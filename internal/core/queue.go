package core

import "sync"

type FrameQueue struct {
	mu         sync.Mutex
	items      []Frame
	head       int
	count      int
	queuedByte int
	maxBytes   int
}

func NewFrameQueue(capacity int, maxBytes int) *FrameQueue {
	return &FrameQueue{
		items:    make([]Frame, capacity),
		maxBytes: maxBytes,
	}
}

func (q *FrameQueue) Push(frame Frame) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	nextBytes := q.queuedByte + len(frame.Payload)
	if nextBytes > q.maxBytes || q.count == len(q.items) {
		return ErrQueueFull
	}
	idx := (q.head + q.count) % len(q.items)
	q.items[idx] = frame
	q.count++
	q.queuedByte = nextBytes
	return nil
}

func (q *FrameQueue) Pop() (Frame, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.count == 0 {
		return Frame{}, false
	}
	item := q.items[q.head]
	q.items[q.head] = Frame{}
	q.head = (q.head + 1) % len(q.items)
	q.count--
	q.queuedByte -= len(item.Payload)
	return item, true
}

func (q *FrameQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.count
}

func (q *FrameQueue) QueuedBytes() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.queuedByte
}
