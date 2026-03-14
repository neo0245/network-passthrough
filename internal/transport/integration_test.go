package transport_test

import (
	"context"
	"io"
	"net"
	"testing"
	"time"

	"slpp/internal/core"
	"slpp/internal/testutil"
	"slpp/internal/transport"
)

func TestTunnelRejectsInvalidAuth(t *testing.T) {
	server := testutil.StartServer(t)
	tunnel, err := transport.DialClientTunnel(context.Background(), core.ClientConfig{
		ServerURL:       server.BaseURL,
		Token:           "bad-token",
		InsecureSkipTLS: true,
	})
	if err != nil {
		t.Fatalf("dial tunnel: %v", err)
	}
	defer tunnel.ReadCloser.Close()
	defer tunnel.WriteCloser.Close()

	codec := core.Codec{}
	authFrame, err := transport.NewAuthFrame("bad-token")
	if err != nil {
		t.Fatalf("new auth frame: %v", err)
	}
	if err := codec.Encode(authFrame, tunnel.WriteCloser); err != nil {
		t.Fatalf("encode auth frame: %v", err)
	}
	frame, err := codec.Decode(tunnel.ReadCloser)
	if err != nil {
		if err == io.EOF {
			return
		}
		t.Fatalf("decode auth reply: %v", err)
	}
	if frame.Type != core.FrameAUTHERR {
		t.Fatalf("unexpected frame type: got %x want %x", frame.Type, core.FrameAUTHERR)
	}
}

func TestTunnelOpenTCPFailureReturnsOpenErr(t *testing.T) {
	server := testutil.StartServer(t)
	tunnel, err := transport.DialClientTunnel(context.Background(), core.ClientConfig{
		ServerURL:       server.BaseURL,
		Token:           server.TokenRecord.Token,
		InsecureSkipTLS: true,
	})
	if err != nil {
		t.Fatalf("dial tunnel: %v", err)
	}
	defer tunnel.ReadCloser.Close()
	defer tunnel.WriteCloser.Close()

	codec := core.Codec{}
	authFrame, err := transport.NewAuthFrame(server.TokenRecord.Token)
	if err != nil {
		t.Fatalf("new auth frame: %v", err)
	}
	if err := codec.Encode(authFrame, tunnel.WriteCloser); err != nil {
		t.Fatalf("encode auth frame: %v", err)
	}
	if frame, err := codec.Decode(tunnel.ReadCloser); err != nil || frame.Type != core.FrameAUTHOK {
		t.Fatalf("expected auth ok, got frame=%#v err=%v", frame, err)
	}
	openFrame := core.Frame{
		Type:      core.FrameOPENTCP,
		ChannelID: 1,
		Payload:   core.EncodeAddress("127.0.0.1:1"),
	}
	if err := codec.Encode(openFrame, tunnel.WriteCloser); err != nil {
		t.Fatalf("encode open frame: %v", err)
	}
	frame, err := codec.Decode(tunnel.ReadCloser)
	if err != nil {
		t.Fatalf("decode open reply: %v", err)
	}
	if frame.Type != core.FrameOPENERR {
		t.Fatalf("unexpected frame type: got %x want %x", frame.Type, core.FrameOPENERR)
	}
}

func TestExpiredUDPDatagramIncrementsStats(t *testing.T) {
	server := testutil.StartServer(t)
	udpAddr := testutil.StartUDPEcho(t)
	tunnel, err := transport.DialClientTunnel(context.Background(), core.ClientConfig{
		ServerURL:       server.BaseURL,
		Token:           server.TokenRecord.Token,
		InsecureSkipTLS: true,
	})
	if err != nil {
		t.Fatalf("dial tunnel: %v", err)
	}
	defer tunnel.ReadCloser.Close()
	defer tunnel.WriteCloser.Close()

	codec := core.Codec{}
	authFrame, err := transport.NewAuthFrame(server.TokenRecord.Token)
	if err != nil {
		t.Fatalf("new auth frame: %v", err)
	}
	if err := codec.Encode(authFrame, tunnel.WriteCloser); err != nil {
		t.Fatalf("encode auth frame: %v", err)
	}
	if _, err := codec.Decode(tunnel.ReadCloser); err != nil {
		t.Fatalf("decode auth ok: %v", err)
	}
	if err := codec.Encode(core.Frame{
		Type:      core.FrameOPENUDP,
		ChannelID: 1,
		Payload:   core.EncodeAddress(udpAddr),
	}, tunnel.WriteCloser); err != nil {
		t.Fatalf("encode open udp frame: %v", err)
	}
	if frame, err := codec.Decode(tunnel.ReadCloser); err != nil || frame.Type != core.FrameOPENOK {
		t.Fatalf("expected open ok, got frame=%#v err=%v", frame, err)
	}

	dg := core.UDPDatagram{
		ID:         1,
		SentMS:     uint32(time.Now().Add(-2 * time.Second).UnixMilli()),
		DeadlineMS: 10,
		FragIndex:  0,
		FragCount:  1,
		TotalLen:   4,
		Data:       []byte("ping"),
	}
	if err := codec.Encode(core.Frame{
		Type:      core.FrameUDPDATA,
		ChannelID: 1,
		Payload:   core.EncodeUDPDatagram(dg),
	}, tunnel.WriteCloser); err != nil {
		t.Fatalf("encode udp data: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if server.Stats.Snapshot().ExpiredUDP > 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("expected expired udp stats to increment")
}

func TestUDPDatagramReachesTarget(t *testing.T) {
	server := testutil.StartServer(t)
	targetConn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen udp target: %v", err)
	}
	defer targetConn.Close()
	received := make(chan []byte, 1)
	go func() {
		buf := make([]byte, 2048)
		n, _, readErr := targetConn.ReadFrom(buf)
		if readErr == nil {
			received <- append([]byte(nil), buf[:n]...)
		}
	}()

	tunnel, err := transport.DialClientTunnel(context.Background(), core.ClientConfig{
		ServerURL:       server.BaseURL,
		Token:           server.TokenRecord.Token,
		InsecureSkipTLS: true,
	})
	if err != nil {
		t.Fatalf("dial tunnel: %v", err)
	}
	defer tunnel.ReadCloser.Close()
	defer tunnel.WriteCloser.Close()

	codec := core.Codec{}
	authFrame, err := transport.NewAuthFrame(server.TokenRecord.Token)
	if err != nil {
		t.Fatalf("new auth frame: %v", err)
	}
	if err := codec.Encode(authFrame, tunnel.WriteCloser); err != nil {
		t.Fatalf("encode auth frame: %v", err)
	}
	if frame, err := codec.Decode(tunnel.ReadCloser); err != nil || frame.Type != core.FrameAUTHOK {
		t.Fatalf("expected auth ok, got frame=%#v err=%v", frame, err)
	}
	if err := codec.Encode(core.Frame{
		Type:      core.FrameOPENUDP,
		ChannelID: 7,
		Payload:   core.EncodeAddress(targetConn.LocalAddr().String()),
	}, tunnel.WriteCloser); err != nil {
		t.Fatalf("encode open udp frame: %v", err)
	}
	if frame, err := codec.Decode(tunnel.ReadCloser); err != nil || frame.Type != core.FrameOPENOK {
		t.Fatalf("expected open ok, got frame=%#v err=%v", frame, err)
	}

	payload := []byte("udp-echo")
	dg := core.UDPDatagram{
		ID:         99,
		SentMS:     uint32(time.Now().UnixMilli()),
		DeadlineMS: 1000,
		FragIndex:  0,
		FragCount:  1,
		TotalLen:   uint16(len(payload)),
		Data:       payload,
	}
	if err := codec.Encode(core.Frame{
		Type:      core.FrameUDPDATA,
		ChannelID: 7,
		Payload:   core.EncodeUDPDatagram(dg),
	}, tunnel.WriteCloser); err != nil {
		t.Fatalf("encode udp data: %v", err)
	}

	select {
	case got := <-received:
		if string(got) == string(payload) {
			return
		}
		t.Fatalf("unexpected udp payload: got %q want %q", got, payload)
	case <-time.After(5 * time.Second):
		t.Fatal("expected udp target to receive payload")
	}
}

func TestTunnelMalformedFrameClosesSession(t *testing.T) {
	server := testutil.StartServer(t)
	tunnel, err := transport.DialClientTunnel(context.Background(), core.ClientConfig{
		ServerURL:       server.BaseURL,
		Token:           server.TokenRecord.Token,
		InsecureSkipTLS: true,
	})
	if err != nil {
		t.Fatalf("dial tunnel: %v", err)
	}
	defer tunnel.ReadCloser.Close()
	defer tunnel.WriteCloser.Close()

	badHeader := []byte{
		0x02, core.FrameAUTH, 0x00, core.HeaderLen,
		0, 0, 0, 1,
		0, 0, 0, 0,
		0, 0, 0, 1,
		0, 0, 0, 0,
	}
	if _, err := tunnel.WriteCloser.Write(badHeader); err != nil {
		t.Fatalf("write bad header: %v", err)
	}
	errCh := make(chan error, 1)
	go func() {
		_, readErr := core.Codec{}.Decode(tunnel.ReadCloser)
		errCh <- readErr
	}()
	select {
	case readErr := <-errCh:
		if readErr == nil {
			t.Fatal("expected session read failure after malformed frame")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for malformed frame failure")
	}
}
