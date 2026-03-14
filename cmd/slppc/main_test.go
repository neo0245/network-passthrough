package main

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net"
	"strconv"
	"testing"
	"time"

	"slpp/internal/core"
	"slpp/internal/testutil"
)

func TestCheckClientMain(t *testing.T) {
	t.Parallel()
	if _, err := checkClientMain("", "127.0.0.1:1080"); err == nil {
		t.Fatal("expected missing server error")
	}
	out, err := checkClientMain("https://127.0.0.1:8443", "127.0.0.1:1080")
	if err != nil {
		t.Fatalf("checkClientMain: %v", err)
	}
	if !out.OK {
		t.Fatalf("expected ok output: %#v", out)
	}
}

func TestPingMain(t *testing.T) {
	server := testutil.StartServer(t)

	out, err := pingMain(context.Background(), server.BaseURL, true)
	if err != nil {
		t.Fatalf("pingMain: %v", err)
	}
	if out.Message != "200 OK" {
		t.Fatalf("unexpected ping output: %#v", out)
	}
	if _, err := pingMain(context.Background(), "https://127.0.0.1:1", true); err == nil {
		t.Fatal("expected unreachable ping error")
	}
}

func TestConnectMainEstablishesSession(t *testing.T) {
	server := testutil.StartServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- connectMain(ctx, core.ClientConfig{
			ServerURL:       server.BaseURL,
			Token:           server.TokenRecord.Token,
			InsecureSkipTLS: true,
		})
	}()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if server.Stats.Snapshot().ActiveSessions > 0 {
			cancel()
			err := <-done
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("expected context cancellation, got %v", err)
			}
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("client session never became active")
}

func TestSocks5MainTCPRelay(t *testing.T) {
	server := testutil.StartServer(t)
	echoAddr := testutil.StartTCPEcho(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	out, err := socks5Main(ctx, socks5Options{
		ServerURL:       server.BaseURL,
		Token:           server.TokenRecord.Token,
		ListenAddr:      "127.0.0.1:0",
		InsecureSkipTLS: true,
	})
	if err != nil {
		t.Fatalf("socks5Main: %v", err)
	}
	listenAddr := envelopeListen(t, out)

	conn, err := net.Dial("tcp", listenAddr)
	if err != nil {
		t.Fatalf("dial socks5 listener: %v", err)
	}
	defer conn.Close()
	if err := testutil.SendSOCKS5Handshake(conn); err != nil {
		t.Fatalf("handshake: %v", err)
	}
	if err := sendSocksConnect(conn, echoAddr); err != nil {
		t.Fatalf("connect request: %v", err)
	}
	msg := []byte("tcp-through-slpp")
	if _, err := conn.Write(msg); err != nil {
		t.Fatalf("write payload: %v", err)
	}
	reply := make([]byte, len(msg))
	if _, err := io.ReadFull(conn, reply); err != nil {
		t.Fatalf("read payload: %v", err)
	}
	if string(reply) != string(msg) {
		t.Fatalf("unexpected tcp echo: got %q want %q", reply, msg)
	}
}

func TestSocks5MainUDPAssociate(t *testing.T) {
	server := testutil.StartServer(t)
	echoAddr := testutil.StartUDPEcho(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	out, err := socks5Main(ctx, socks5Options{
		ServerURL:       server.BaseURL,
		Token:           server.TokenRecord.Token,
		ListenAddr:      "127.0.0.1:0",
		InsecureSkipTLS: true,
	})
	if err != nil {
		t.Fatalf("socks5Main: %v", err)
	}
	listenAddr := envelopeListen(t, out)

	tcpConn, err := net.Dial("tcp", listenAddr)
	if err != nil {
		t.Fatalf("dial socks5 listener: %v", err)
	}
	defer tcpConn.Close()
	if err := testutil.SendSOCKS5Handshake(tcpConn); err != nil {
		t.Fatalf("handshake: %v", err)
	}
	udpRelayAddr, err := sendUDPAssociate(tcpConn)
	if err != nil {
		t.Fatalf("udp associate: %v", err)
	}

	udpConn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Fatalf("listen udp client: %v", err)
	}
	defer udpConn.Close()
	relayAddr, err := net.ResolveUDPAddr("udp", udpRelayAddr)
	if err != nil {
		t.Fatalf("resolve udp relay: %v", err)
	}

	payload := []byte("udp-through-slpp")
	packet, err := encodeSocksUDPDatagram(echoAddr, payload)
	if err != nil {
		t.Fatalf("encode udp packet: %v", err)
	}
	if _, err := udpConn.WriteToUDP(packet, relayAddr); err != nil {
		t.Fatalf("write udp packet: %v", err)
	}
	if err := udpConn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	buf := make([]byte, 2048)
	n, _, err := udpConn.ReadFromUDP(buf)
	if err != nil {
		t.Fatalf("read udp response: %v", err)
	}
	_, headerLen, err := readPacketAddr(buf[3:n])
	if err != nil {
		t.Fatalf("read packet addr: %v", err)
	}
	got := buf[3+headerLen : n]
	if string(got) != string(payload) {
		t.Fatalf("unexpected udp echo: got %q want %q", got, payload)
	}
}

func envelopeListen(t *testing.T, out interface{}) string {
	t.Helper()
	raw, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	var env struct {
		Data map[string]string `json:"data"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	return env.Data["listen"]
}

func sendSocksConnect(conn net.Conn, target string) error {
	host, portText, err := net.SplitHostPort(target)
	if err != nil {
		return err
	}
	ip := net.ParseIP(host).To4()
	if ip == nil {
		return errors.New("only ipv4 supported in test helper")
	}
	port, err := net.LookupPort("tcp", portText)
	if err != nil {
		return err
	}
	req := []byte{0x05, 0x01, 0x00, 0x01}
	req = append(req, ip...)
	var portBuf [2]byte
	binary.BigEndian.PutUint16(portBuf[:], uint16(port))
	req = append(req, portBuf[:]...)
	if _, err := conn.Write(req); err != nil {
		return err
	}
	reply := make([]byte, 10)
	if _, err := io.ReadFull(conn, reply); err != nil {
		return err
	}
	if reply[1] != 0x00 {
		return errors.New("connect rejected")
	}
	return nil
}

func sendUDPAssociate(conn net.Conn) (string, error) {
	req := []byte{0x05, 0x03, 0x00, 0x01, 0, 0, 0, 0, 0, 0}
	if _, err := conn.Write(req); err != nil {
		return "", err
	}
	reply := make([]byte, 10)
	if _, err := io.ReadFull(conn, reply); err != nil {
		return "", err
	}
	if reply[1] != 0x00 {
		return "", errors.New("udp associate rejected")
	}
	host := net.IP(reply[4:8]).String()
	port := binary.BigEndian.Uint16(reply[8:10])
	return net.JoinHostPort(host, strconv.Itoa(int(port))), nil
}
