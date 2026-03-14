package main

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"slpp/internal/core"
	"slpp/internal/transport"
	"slpp/pkg/api"
)

const version = "0.1.0"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "connect":
		connectCmd(os.Args[2:])
	case "socks5":
		socks5Cmd(os.Args[2:])
	case "check":
		checkCmd(os.Args[2:])
	case "ping":
		pingCmd(os.Args[2:])
	case "stats":
		statsCmd(os.Args[2:])
	case "version":
		fmt.Println(version)
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, "usage: slppc {connect|socks5|check|ping|stats|version}\n")
}

func connectCmd(args []string) {
	fs := flag.NewFlagSet("connect", flag.ExitOnError)
	serverURL := fs.String("server", "", "server URL")
	token := fs.String("token", "", "bearer token")
	jsonOut := fs.Bool("json", false, "json output")
	insecure := fs.Bool("insecure", false, "skip tls verification")
	fs.Parse(args)
	if *serverURL == "" || *token == "" {
		exitErr(errors.New("--server and --token are required"), *jsonOut)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	tunnel, err := transport.DialClientTunnel(ctx, core.ClientConfig{
		ServerURL:       *serverURL,
		Token:           *token,
		InsecureSkipTLS: *insecure,
	})
	if err != nil {
		exitErr(err, *jsonOut)
	}
	defer tunnel.ReadCloser.Close()
	defer tunnel.WriteCloser.Close()
	sess := core.NewSession(uint32(time.Now().UnixNano()), tunnel.ReadCloser, tunnel.WriteCloser, &noopHandler{}, core.NewStats(), transport.DefaultHeartbeat())
	authFrame, err := transport.NewAuthFrame(*token)
	if err != nil {
		exitErr(err, *jsonOut)
	}
	if err := sess.QueueControl(authFrame); err != nil {
		exitErr(err, *jsonOut)
	}
	runCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() { _ = sess.Run(runCtx) }()
	printEnvelope(api.OutputEnvelope{OK: true, Message: "tunnel connected"}, *jsonOut)
	<-runCtx.Done()
}

type tunnelClient struct {
	session     *core.Session
	tcpMu       sync.Mutex
	tcpConns    map[uint32]net.Conn
	udpMu       sync.Mutex
	udpRoutes   map[uint32]udpRoute
	openMu      sync.Mutex
	openResults map[uint32]chan error
	nextID      atomic.Uint32
}

type udpRoute struct {
	conn   *net.UDPConn
	peer   *net.UDPAddr
	target string
}

func newTunnelClient(session *core.Session) *tunnelClient {
	c := &tunnelClient{
		session:     session,
		tcpConns:    make(map[uint32]net.Conn),
		udpRoutes:   make(map[uint32]udpRoute),
		openResults: make(map[uint32]chan error),
	}
	c.nextID.Store(1)
	return c
}

func (c *tunnelClient) nextChannel() uint32 {
	return c.nextID.Add(1)
}

func (c *tunnelClient) HandleFrame(ctx context.Context, s *core.Session, frame core.Frame) error {
	switch frame.Type {
	case core.FrameAUTHOK:
		return nil
	case core.FrameAUTHERR:
		return errors.New(string(frame.Payload))
	case core.FramePING:
		return s.QueueControl(core.Frame{Type: core.FramePONG})
	case core.FrameOPENOK:
		c.signalOpen(frame.ChannelID, nil)
		return nil
	case core.FrameOPENERR:
		c.signalOpen(frame.ChannelID, errors.New(string(frame.Payload)))
		return nil
	case core.FrameTCPDATA:
		c.tcpMu.Lock()
		conn := c.tcpConns[frame.ChannelID]
		c.tcpMu.Unlock()
		if conn != nil {
			_, err := conn.Write(frame.Payload)
			return err
		}
	case core.FrameTCPCLOSE:
		c.closeTCP(frame.ChannelID)
	case core.FrameUDPDATA:
		dg, err := core.DecodeUDPDatagram(frame.Payload)
		if err != nil {
			return err
		}
		c.udpMu.Lock()
		route := c.udpRoutes[frame.ChannelID]
		c.udpMu.Unlock()
		if route.conn != nil && route.peer != nil {
			packet, packetErr := encodeSocksUDPDatagram(route.target, dg.Data)
			if packetErr != nil {
				return packetErr
			}
			_, err := route.conn.WriteToUDP(packet, route.peer)
			return err
		}
	case core.FrameUDPCLOSE:
		c.closeUDP(frame.ChannelID)
	}
	return nil
}

func (c *tunnelClient) signalOpen(channelID uint32, err error) {
	c.openMu.Lock()
	ch := c.openResults[channelID]
	delete(c.openResults, channelID)
	c.openMu.Unlock()
	if ch != nil {
		ch <- err
		close(ch)
	}
}

func (c *tunnelClient) closeTCP(channelID uint32) {
	c.tcpMu.Lock()
	conn := c.tcpConns[channelID]
	delete(c.tcpConns, channelID)
	c.tcpMu.Unlock()
	if conn != nil {
		_ = conn.Close()
	}
}

func (c *tunnelClient) closeUDP(channelID uint32) {
	c.udpMu.Lock()
	delete(c.udpRoutes, channelID)
	c.udpMu.Unlock()
}

func (c *tunnelClient) openTCPChannel(address string, conn net.Conn) (uint32, error) {
	channelID := c.nextChannel()
	resCh := make(chan error, 1)
	c.tcpMu.Lock()
	c.tcpConns[channelID] = conn
	c.tcpMu.Unlock()
	c.openMu.Lock()
	c.openResults[channelID] = resCh
	c.openMu.Unlock()
	if err := c.session.QueueControl(core.Frame{
		Type:      core.FrameOPENTCP,
		ChannelID: channelID,
		Payload:   core.EncodeAddress(address),
	}); err != nil {
		return 0, err
	}
	select {
	case err := <-resCh:
		return channelID, err
	case <-time.After(10 * time.Second):
		return 0, errors.New("open timeout")
	}
}

func (c *tunnelClient) openUDPChannel(address string, conn *net.UDPConn, peer *net.UDPAddr) (uint32, error) {
	channelID := c.nextChannel()
	resCh := make(chan error, 1)
	c.udpMu.Lock()
	c.udpRoutes[channelID] = udpRoute{conn: conn, peer: peer, target: address}
	c.udpMu.Unlock()
	c.openMu.Lock()
	c.openResults[channelID] = resCh
	c.openMu.Unlock()
	if err := c.session.QueueControl(core.Frame{
		Type:      core.FrameOPENUDP,
		ChannelID: channelID,
		Payload:   core.EncodeAddress(address),
	}); err != nil {
		return 0, err
	}
	select {
	case err := <-resCh:
		return channelID, err
	case <-time.After(10 * time.Second):
		return 0, errors.New("udp open timeout")
	}
}

func socks5Cmd(args []string) {
	fs := flag.NewFlagSet("socks5", flag.ExitOnError)
	serverURL := fs.String("server", "", "server URL")
	token := fs.String("token", "", "bearer token")
	listen := fs.String("listen", "127.0.0.1:1080", "socks5 listen address")
	jsonOut := fs.Bool("json", false, "json output")
	insecure := fs.Bool("insecure", false, "skip tls verification")
	fs.Parse(args)
	if *serverURL == "" || *token == "" {
		exitErr(errors.New("--server and --token are required"), *jsonOut)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	tunnel, err := transport.DialClientTunnel(ctx, core.ClientConfig{
		ServerURL:       *serverURL,
		Token:           *token,
		InsecureSkipTLS: *insecure,
	})
	if err != nil {
		exitErr(err, *jsonOut)
	}
	client := newTunnelClient(nil)
	session := core.NewSession(uint32(time.Now().UnixNano()), tunnel.ReadCloser, tunnel.WriteCloser, client, core.NewStats(), transport.DefaultHeartbeat())
	client.session = session
	authFrame, err := transport.NewAuthFrame(*token)
	if err != nil {
		exitErr(err, *jsonOut)
	}
	if err := session.QueueControl(authFrame); err != nil {
		exitErr(err, *jsonOut)
	}
	go func() { _ = session.Run(ctx) }()
	ln, err := net.Listen("tcp", *listen)
	if err != nil {
		exitErr(err, *jsonOut)
	}
	defer ln.Close()
	printEnvelope(api.OutputEnvelope{OK: true, Message: "SOCKS5 listening", Data: map[string]string{"listen": *listen}}, *jsonOut)
	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			continue
		}
		go handleSocksConn(ctx, conn, client)
	}
}

func handleSocksConn(ctx context.Context, conn net.Conn, client *tunnelClient) {
	defer conn.Close()
	buf := make([]byte, 262)
	if _, err := io.ReadFull(conn, buf[:2]); err != nil {
		return
	}
	nmethods := int(buf[1])
	if _, err := io.ReadFull(conn, buf[:nmethods]); err != nil {
		return
	}
	if _, err := conn.Write([]byte{0x05, 0x00}); err != nil {
		return
	}
	if _, err := io.ReadFull(conn, buf[:4]); err != nil {
		return
	}
	cmd := buf[1]
	atyp := buf[3]
	addr, rawAddr, err := readSocksAddr(conn, atyp)
	if err != nil {
		return
	}
	switch cmd {
	case 0x01:
		channelID, err := client.openTCPChannel(addr, conn)
		if err != nil {
			_, _ = conn.Write([]byte{0x05, 0x01, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
			return
		}
		_, _ = conn.Write(append([]byte{0x05, 0x00, 0x00}, rawAddr...))
		copyTCPToTunnel(ctx, conn, client, channelID)
	case 0x03:
		udpAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
		if err != nil {
			return
		}
		udpConn, err := net.ListenUDP("udp", udpAddr)
		if err != nil {
			return
		}
		bound := udpConn.LocalAddr().(*net.UDPAddr)
		respAddr := socksIPv4Reply(bound.IP, uint16(bound.Port))
		if _, err := conn.Write(append([]byte{0x05, 0x00, 0x00}, respAddr...)); err != nil {
			_ = udpConn.Close()
			return
		}
		go handleUDPAssociate(ctx, conn, udpConn, client)
		<-ctx.Done()
	default:
		_, _ = conn.Write([]byte{0x05, 0x07, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
	}
}

func readSocksAddr(r io.Reader, atyp byte) (string, []byte, error) {
	switch atyp {
	case 0x01:
		buf := make([]byte, 6)
		if _, err := io.ReadFull(r, buf); err != nil {
			return "", nil, err
		}
		host := net.IP(buf[:4]).String()
		port := binary.BigEndian.Uint16(buf[4:6])
		raw := append([]byte{0x01}, buf...)
		return net.JoinHostPort(host, fmt.Sprintf("%d", port)), raw, nil
	case 0x03:
		var size [1]byte
		if _, err := io.ReadFull(r, size[:]); err != nil {
			return "", nil, err
		}
		buf := make([]byte, int(size[0])+2)
		if _, err := io.ReadFull(r, buf); err != nil {
			return "", nil, err
		}
		host := string(buf[:len(buf)-2])
		port := binary.BigEndian.Uint16(buf[len(buf)-2:])
		raw := append([]byte{0x03, size[0]}, buf...)
		return net.JoinHostPort(host, fmt.Sprintf("%d", port)), raw, nil
	case 0x04:
		buf := make([]byte, 18)
		if _, err := io.ReadFull(r, buf); err != nil {
			return "", nil, err
		}
		host := net.IP(buf[:16]).String()
		port := binary.BigEndian.Uint16(buf[16:18])
		raw := append([]byte{0x04}, buf...)
		return net.JoinHostPort(host, fmt.Sprintf("%d", port)), raw, nil
	default:
		return "", nil, errors.New("unsupported atyp")
	}
}

func copyTCPToTunnel(ctx context.Context, conn net.Conn, client *tunnelClient, channelID uint32) {
	buf := make([]byte, 16*1024)
	for {
		n, err := conn.Read(buf)
		if n > 0 {
			_ = client.session.QueueData(core.Frame{
				Type:      core.FrameTCPDATA,
				ChannelID: channelID,
				Payload:   append([]byte(nil), buf[:n]...),
			})
		}
		if err != nil {
			_ = client.session.QueueControl(core.Frame{Type: core.FrameTCPCLOSE, ChannelID: channelID})
			client.closeTCP(channelID)
			return
		}
		select {
		case <-ctx.Done():
			return
		default:
		}
	}
}

func handleUDPAssociate(ctx context.Context, tcpConn net.Conn, udpConn *net.UDPConn, client *tunnelClient) {
	defer udpConn.Close()
	buf := make([]byte, 2048)
	channelForRemote := make(map[string]uint32)
	for {
		n, peer, err := udpConn.ReadFromUDP(buf)
		if err != nil {
			return
		}
		if n < 10 {
			continue
		}
		atyp := buf[3]
		addr, _, err := readPacketAddr(buf[3:n])
		if err != nil {
			continue
		}
		headerLen := packetAddrLen(buf[3:n], atyp)
		if headerLen <= 0 || headerLen > n {
			continue
		}
		payload := append([]byte(nil), buf[headerLen:n]...)
		if len(payload) > core.MaxLocalUDPDatagram {
			payload = payload[:core.MaxLocalUDPDatagram]
		}
		key := peer.String() + "|" + addr
		channelID, ok := channelForRemote[key]
		if !ok {
			channelID, err = client.openUDPChannel(addr, udpConn, peer)
			if err != nil {
				continue
			}
			channelForRemote[key] = channelID
		}
		dg := core.UDPDatagram{
			ID:         uint32(time.Now().UnixNano()),
			SentMS:     uint32(time.Now().UnixMilli()),
			DeadlineMS: 1000,
			FragIndex:  0,
			FragCount:  1,
			TotalLen:   uint16(len(payload)),
			Data:       payload,
		}
		_ = client.session.QueueData(core.Frame{
			Type:      core.FrameUDPDATA,
			ChannelID: channelID,
			Payload:   core.EncodeUDPDatagram(dg),
		})
		_ = tcpConn.SetReadDeadline(time.Now().Add(30 * time.Second))
	}
}

func readPacketAddr(packet []byte) (string, int, error) {
	if len(packet) < 1 {
		return "", 0, io.ErrUnexpectedEOF
	}
	switch packet[0] {
	case 0x01:
		if len(packet) < 7 {
			return "", 0, io.ErrUnexpectedEOF
		}
		host := net.IP(packet[1:5]).String()
		port := binary.BigEndian.Uint16(packet[5:7])
		return net.JoinHostPort(host, fmt.Sprintf("%d", port)), 7, nil
	case 0x03:
		if len(packet) < 2 {
			return "", 0, io.ErrUnexpectedEOF
		}
		size := int(packet[1])
		if len(packet) < 2+size+2 {
			return "", 0, io.ErrUnexpectedEOF
		}
		host := string(packet[2 : 2+size])
		port := binary.BigEndian.Uint16(packet[2+size : 2+size+2])
		return net.JoinHostPort(host, fmt.Sprintf("%d", port)), 2 + size + 2, nil
	case 0x04:
		if len(packet) < 19 {
			return "", 0, io.ErrUnexpectedEOF
		}
		host := net.IP(packet[1:17]).String()
		port := binary.BigEndian.Uint16(packet[17:19])
		return net.JoinHostPort(host, fmt.Sprintf("%d", port)), 19, nil
	default:
		return "", 0, errors.New("unsupported atyp")
	}
}

func packetAddrLen(packet []byte, atyp byte) int {
	_, n, err := readPacketAddr(packet)
	if err != nil {
		return 0
	}
	return 3 + n
}

func encodeSocksUDPDatagram(target string, payload []byte) ([]byte, error) {
	host, portText, err := net.SplitHostPort(target)
	if err != nil {
		return nil, err
	}
	portNum, err := net.LookupPort("udp", portText)
	if err != nil {
		return nil, err
	}
	packet := []byte{0x00, 0x00, 0x00}
	if ip := net.ParseIP(host); ip != nil {
		if v4 := ip.To4(); v4 != nil {
			packet = append(packet, 0x01)
			packet = append(packet, v4...)
		} else {
			packet = append(packet, 0x04)
			packet = append(packet, ip.To16()...)
		}
	} else {
		if len(host) > 255 {
			return nil, errors.New("hostname too long")
		}
		packet = append(packet, 0x03, byte(len(host)))
		packet = append(packet, host...)
	}
	var portBuf [2]byte
	binary.BigEndian.PutUint16(portBuf[:], uint16(portNum))
	packet = append(packet, portBuf[:]...)
	packet = append(packet, payload...)
	return packet, nil
}

func socksIPv4Reply(ip net.IP, port uint16) []byte {
	if v4 := ip.To4(); v4 != nil {
		reply := make([]byte, 0, 7)
		reply = append(reply, 0x01)
		reply = append(reply, v4...)
		var portBuf [2]byte
		binary.BigEndian.PutUint16(portBuf[:], port)
		reply = append(reply, portBuf[:]...)
		return reply
	}
	return []byte{0x01, 0, 0, 0, 0, 0, 0}
}

func checkCmd(args []string) {
	fs := flag.NewFlagSet("check", flag.ExitOnError)
	serverURL := fs.String("server", "", "server URL")
	listen := fs.String("listen", "127.0.0.1:1080", "listen address")
	jsonOut := fs.Bool("json", false, "json output")
	fs.Parse(args)
	if *serverURL == "" {
		exitErr(errors.New("--server is required"), *jsonOut)
	}
	if _, err := url.Parse(*serverURL); err != nil {
		exitErr(err, *jsonOut)
	}
	printEnvelope(api.OutputEnvelope{OK: true, Message: "configuration valid", Data: map[string]string{"server": *serverURL, "listen": *listen}}, *jsonOut)
}

func pingCmd(args []string) {
	fs := flag.NewFlagSet("ping", flag.ExitOnError)
	serverURL := fs.String("server", "", "server URL")
	jsonOut := fs.Bool("json", false, "json output")
	insecure := fs.Bool("insecure", false, "skip tls verification")
	fs.Parse(args)
	if *serverURL == "" {
		exitErr(errors.New("--server is required"), *jsonOut)
	}
	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig:  transportTLSConfig(*insecure),
			ForceAttemptHTTP2: true,
		},
	}
	resp, err := client.Get(*serverURL + "/healthz")
	if err != nil {
		exitErr(err, *jsonOut)
	}
	defer resp.Body.Close()
	printEnvelope(api.OutputEnvelope{OK: true, Message: resp.Status}, *jsonOut)
}

func statsCmd(args []string) {
	fs := flag.NewFlagSet("stats", flag.ExitOnError)
	jsonOut := fs.Bool("json", false, "json output")
	fs.Parse(args)
	printEnvelope(api.OutputEnvelope{OK: true, Data: core.NewStats().Snapshot()}, *jsonOut)
}

type noopHandler struct{}

func (noopHandler) HandleFrame(ctx context.Context, s *core.Session, frame core.Frame) error {
	if frame.Type == core.FramePING {
		return s.QueueControl(core.Frame{Type: core.FramePONG})
	}
	if frame.Type == core.FrameAUTHERR {
		return errors.New(string(frame.Payload))
	}
	return nil
}

func transportTLSConfig(insecure bool) *tls.Config {
	return &tls.Config{
		MinVersion:         tls.VersionTLS13,
		InsecureSkipVerify: insecure,
		NextProtos:         []string{"h2"},
	}
}

func printEnvelope(out api.OutputEnvelope, asJSON bool) {
	if asJSON {
		_ = json.NewEncoder(os.Stdout).Encode(out)
		return
	}
	if out.Message != "" {
		fmt.Println(out.Message)
	}
	if out.Data != nil {
		data, _ := json.MarshalIndent(out.Data, "", "  ")
		fmt.Println(string(data))
	}
}

func exitErr(err error, asJSON bool) {
	if asJSON {
		_ = json.NewEncoder(os.Stdout).Encode(api.OutputEnvelope{OK: false, Message: err.Error()})
	} else {
		fmt.Fprintln(os.Stderr, "error:", err)
	}
	os.Exit(1)
}
