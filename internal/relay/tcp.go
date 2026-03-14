package relay

import (
	"context"
	"io"
	"net"
	"sync"

	"slpp/internal/core"
)

type TCPManager interface {
	Open(context.Context, uint32, string) error
	Register(uint32, net.Conn)
	Close(uint32)
	Write(uint32, []byte) error
}

type TCPRelay struct {
	mu      sync.Mutex
	conns   map[uint32]net.Conn
	session *core.Session
}

func NewTCPRelay(session *core.Session) *TCPRelay {
	return &TCPRelay{
		conns:   make(map[uint32]net.Conn),
		session: session,
	}
}

func (r *TCPRelay) Open(ctx context.Context, channelID uint32, address string) error {
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", address)
	if err != nil {
		return err
	}
	r.Register(channelID, conn)
	go r.copyRemoteToTunnel(channelID, conn)
	return nil
}

func (r *TCPRelay) Register(channelID uint32, conn net.Conn) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.conns[channelID] = conn
}

func (r *TCPRelay) Close(channelID uint32) {
	r.mu.Lock()
	conn := r.conns[channelID]
	delete(r.conns, channelID)
	r.mu.Unlock()
	if conn != nil {
		_ = conn.Close()
	}
}

func (r *TCPRelay) Write(channelID uint32, data []byte) error {
	r.mu.Lock()
	conn := r.conns[channelID]
	r.mu.Unlock()
	if conn == nil {
		return io.EOF
	}
	_, err := conn.Write(data)
	return err
}

func (r *TCPRelay) copyRemoteToTunnel(channelID uint32, conn net.Conn) {
	buf := make([]byte, 16*1024)
	for {
		n, err := conn.Read(buf)
		if n > 0 {
			_ = r.session.QueueData(core.Frame{
				Type:      core.FrameTCPDATA,
				ChannelID: channelID,
				Payload:   append([]byte(nil), buf[:n]...),
			})
		}
		if err != nil {
			_ = r.session.QueueControl(core.Frame{Type: core.FrameTCPCLOSE, ChannelID: channelID})
			r.Close(channelID)
			return
		}
	}
}
