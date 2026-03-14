package relay

import (
	"context"
	"io"
	"net"
	"sync"
	"time"

	"slpp/internal/core"
)

type UDPManager interface {
	Open(context.Context, uint32, string) error
	Close(uint32)
	Write(uint32, core.UDPDatagram) error
}

type udpAssoc struct {
	conn    net.Conn
	expires time.Time
}

type UDPRelay struct {
	mu          sync.Mutex
	assocs      map[uint32]udpAssoc
	session     *core.Session
	reassembler *core.Reassembler
}

func NewUDPRelay(session *core.Session) *UDPRelay {
	return &UDPRelay{
		assocs:      make(map[uint32]udpAssoc),
		session:     session,
		reassembler: core.NewReassembler(),
	}
}

func (r *UDPRelay) Open(ctx context.Context, channelID uint32, address string) error {
	conn, err := (&net.Dialer{}).DialContext(ctx, "udp", address)
	if err != nil {
		return err
	}
	r.mu.Lock()
	r.assocs[channelID] = udpAssoc{conn: conn, expires: time.Now().Add(time.Minute)}
	r.mu.Unlock()
	go r.readLoop(channelID, conn)
	return nil
}

func (r *UDPRelay) Close(channelID uint32) {
	r.mu.Lock()
	assoc := r.assocs[channelID]
	delete(r.assocs, channelID)
	r.mu.Unlock()
	if assoc.conn != nil {
		_ = assoc.conn.Close()
	}
}

func (r *UDPRelay) Write(channelID uint32, dg core.UDPDatagram) error {
	r.mu.Lock()
	assoc := r.assocs[channelID]
	r.mu.Unlock()
	if assoc.conn == nil {
		return io.EOF
	}
	_, err := assoc.conn.Write(dg.Data)
	return err
}

func (r *UDPRelay) readLoop(channelID uint32, conn net.Conn) {
	buf := make([]byte, core.MaxLocalUDPDatagram)
	for {
		n, err := conn.Read(buf)
		if n > 0 {
			frags, fragErr := core.FragmentUDP(buf[:n], uint32(time.Now().UnixNano()), 1000)
			if fragErr == nil {
				for _, frag := range frags {
					_ = r.session.QueueData(core.Frame{
						Type:      core.FrameUDPDATA,
						ChannelID: channelID,
						Payload:   core.EncodeUDPDatagram(frag),
					})
				}
			}
		}
		if err != nil {
			_ = r.session.QueueControl(core.Frame{Type: core.FrameUDPCLOSE, ChannelID: channelID})
			r.Close(channelID)
			return
		}
	}
}
