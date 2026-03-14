package core

import (
	"context"
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"time"
)

type SessionHandler interface {
	HandleFrame(context.Context, *Session, Frame) error
}

type SessionEngine interface {
	Run(context.Context) error
	QueueControl(Frame) error
	QueueData(Frame) error
	Stats() *Stats
}

type Session struct {
	id         uint32
	codec      FrameCodec
	reader     io.Reader
	writer     io.Writer
	handler    SessionHandler
	stats      *Stats
	heartbeat  time.Duration
	controlQ   *FrameQueue
	dataQ      *FrameQueue
	closeOnce  sync.Once
	closed     chan struct{}
	errMu      sync.Mutex
	lastErr    error
	nextSeq    atomic.Uint32
	lastActive atomic.Int64
}

func NewSession(id uint32, reader io.Reader, writer io.Writer, handler SessionHandler, stats *Stats, heartbeat time.Duration) *Session {
	if heartbeat <= 0 {
		heartbeat = DefaultHeartbeatSecs * time.Second
	}
	s := &Session{
		id:        id,
		codec:     Codec{},
		reader:    reader,
		writer:    writer,
		handler:   handler,
		stats:     stats,
		heartbeat: heartbeat,
		controlQ:  NewFrameQueue(256, MaxQueuedBytesSession),
		dataQ:     NewFrameQueue(1024, MaxQueuedBytesSession),
		closed:    make(chan struct{}),
	}
	s.lastActive.Store(time.Now().UnixNano())
	return s
}

func (s *Session) Run(ctx context.Context) error {
	errCh := make(chan error, 3)
	go func() { errCh <- s.readLoop(ctx) }()
	go func() { errCh <- s.writeLoop(ctx) }()
	go func() { errCh <- s.heartbeatLoop(ctx) }()
	select {
	case <-ctx.Done():
		s.Close(ctx.Err())
	case err := <-errCh:
		s.Close(err)
	}
	<-s.closed
	return s.Err()
}

func (s *Session) readLoop(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		frame, err := s.codec.Decode(s.reader)
		if err != nil {
			return err
		}
		s.lastActive.Store(time.Now().UnixNano())
		s.stats.framesIn.Add(1)
		s.stats.bytesIn.Add(uint64(len(frame.Payload)))
		if err := s.handler.HandleFrame(ctx, s, frame); err != nil {
			return err
		}
	}
}

func (s *Session) writeLoop(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-s.closed:
			return nil
		default:
		}
		if frame, ok := s.controlQ.Pop(); ok {
			if err := s.writeFrame(frame); err != nil {
				return err
			}
			continue
		}
		if frame, ok := s.dataQ.Pop(); ok {
			if err := s.writeFrame(frame); err != nil {
				return err
			}
			continue
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func (s *Session) heartbeatLoop(ctx context.Context) error {
	ticker := time.NewTicker(s.heartbeat)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-s.closed:
			return nil
		case <-ticker.C:
			frame := Frame{Type: FramePING, SessionID: s.id, Seq: s.nextSeq.Add(1)}
			if err := s.QueueControl(frame); err != nil {
				return err
			}
		}
	}
}

func (s *Session) writeFrame(frame Frame) error {
	if frame.SessionID == 0 {
		frame.SessionID = s.id
	}
	if frame.Seq == 0 && frame.Type != FramePING && frame.Type != FramePONG {
		frame.Seq = s.nextSeq.Add(1)
	}
	if err := s.codec.Encode(frame, s.writer); err != nil {
		return err
	}
	s.stats.framesOut.Add(1)
	s.stats.bytesOut.Add(uint64(len(frame.Payload)))
	return nil
}

func (s *Session) QueueControl(frame Frame) error {
	err := s.controlQ.Push(frame)
	s.stats.SetSessionQueueBytes(uint64(s.controlQ.QueuedBytes() + s.dataQ.QueuedBytes()))
	return err
}

func (s *Session) QueueData(frame Frame) error {
	err := s.dataQ.Push(frame)
	s.stats.SetSessionQueueBytes(uint64(s.controlQ.QueuedBytes() + s.dataQ.QueuedBytes()))
	return err
}

func (s *Session) Close(err error) {
	s.closeOnce.Do(func() {
		s.errMu.Lock()
		s.lastErr = err
		s.errMu.Unlock()
		close(s.closed)
	})
}

func (s *Session) Err() error {
	s.errMu.Lock()
	defer s.errMu.Unlock()
	return s.lastErr
}

func (s *Session) Stats() *Stats {
	return s.stats
}

func (s *Session) LastActive() time.Time {
	return time.Unix(0, s.lastActive.Load())
}

func IsGraceful(err error) bool {
	return err == nil || errors.Is(err, io.EOF) || errors.Is(err, context.Canceled)
}
