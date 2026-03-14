package testutil

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
	"io"

	"slpp/internal/auth"
	"slpp/internal/control"
	"slpp/internal/core"
	"slpp/internal/transport"
)

type RunningServer struct {
	BaseURL       string
	ControlSocket string
	Stats         *core.Stats
	TokenRecord   auth.TokenRecord
	cancel        context.CancelFunc
	httpServer    *http.Server
}

func GenerateSelfSignedCert(tb testing.TB, dir string) (string, string) {
	tb.Helper()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		tb.Fatalf("generate rsa key: %v", err)
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "127.0.0.1",
		},
		NotBefore:   time.Now().Add(-time.Hour),
		NotAfter:    time.Now().Add(24 * time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
		DNSNames:    []string{"localhost"},
	}

	der, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		tb.Fatalf("create cert: %v", err)
	}

	certFile := filepath.Join(dir, "server.crt")
	keyFile := filepath.Join(dir, "server.key")

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)})

	if err := os.WriteFile(certFile, certPEM, 0o600); err != nil {
		tb.Fatalf("write cert: %v", err)
	}
	if err := os.WriteFile(keyFile, keyPEM, 0o600); err != nil {
		tb.Fatalf("write key: %v", err)
	}
	return certFile, keyFile
}

func GenerateTokenStore(tb testing.TB, path string) auth.TokenRecord {
	tb.Helper()
	store, err := auth.NewStore(path)
	if err != nil {
		tb.Fatalf("create token store: %v", err)
	}
	rec, err := store.Generate(time.Hour)
	if err != nil {
		tb.Fatalf("generate token: %v", err)
	}
	return rec
}

func StartServer(tb testing.TB) *RunningServer {
	tb.Helper()
	dir := tb.TempDir()
	certFile, keyFile := GenerateSelfSignedCert(tb, dir)
	tokenFile := filepath.Join(dir, "tokens.json")
	token := GenerateTokenStore(tb, tokenFile)
	controlSocket := filepath.Join(dir, "slppd.sock")

	stats := core.NewStats()
	store, err := auth.NewStore(tokenFile)
	if err != nil {
		tb.Fatalf("load token store: %v", err)
	}
	handler := &transport.TunnelHandler{Validator: store, Stats: stats}
	httpServer := transport.NewHTTPServer(core.ServerConfig{
		ListenAddr:    "127.0.0.1:0",
		CertFile:      certFile,
		KeyFile:       keyFile,
		ControlSocket: controlSocket,
	}, handler)

	ctx, cancel := context.WithCancel(context.Background())
	controlServer := control.NewServer(controlSocket, stats)
	go func() {
		_ = controlServer.Run(ctx)
	}()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		cancel()
		tb.Fatalf("listen: %v", err)
	}
	go func() {
		_ = httpServer.ServeTLS(ln, certFile, keyFile)
	}()

	baseURL := "https://" + ln.Addr().String()
	waitForHTTPS(tb, baseURL+"/healthz")

	tb.Cleanup(func() {
		cancel()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		_ = httpServer.Shutdown(shutdownCtx)
	})

	return &RunningServer{
		BaseURL:       baseURL,
		ControlSocket: controlSocket,
		Stats:         stats,
		TokenRecord:   token,
		cancel:        cancel,
		httpServer:    httpServer,
	}
}

func waitForHTTPS(tb testing.TB, endpoint string) {
	tb.Helper()
	client := &http.Client{
		Timeout: time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				MinVersion:         tls.VersionTLS13,
				InsecureSkipVerify: true,
				NextProtos:         []string{"h2"},
			},
			ForceAttemptHTTP2: true,
		},
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := client.Get(endpoint)
		if err == nil {
			_ = resp.Body.Close()
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	tb.Fatalf("server did not become healthy: %s", endpoint)
}

func StartTCPEcho(tb testing.TB) string {
	tb.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		tb.Fatalf("tcp echo listen: %v", err)
	}
	tb.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				buf := make([]byte, 32*1024)
				for {
					n, err := c.Read(buf)
					if n > 0 {
						_, _ = c.Write(buf[:n])
					}
					if err != nil {
						return
					}
				}
			}(conn)
		}
	}()
	return ln.Addr().String()
}

func StartUDPEcho(tb testing.TB) string {
	tb.Helper()
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		tb.Fatalf("udp echo listen: %v", err)
	}
	tb.Cleanup(func() { _ = conn.Close() })
	go func() {
		buf := make([]byte, 2048)
		for {
			n, addr, err := conn.ReadFrom(buf)
			if err != nil {
				return
			}
			_, _ = conn.WriteTo(buf[:n], addr)
		}
	}()
	return conn.LocalAddr().String()
}

func FetchControlStats(ctx context.Context, socket string) (*http.Response, error) {
	client := &http.Client{
		Timeout: time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "unix", socket)
			},
		},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://unix/stats", nil)
	if err != nil {
		return nil, err
	}
	return client.Do(req)
}

func SendSOCKS5Handshake(conn net.Conn) error {
	if _, err := conn.Write([]byte{0x05, 0x01, 0x00}); err != nil {
		return err
	}
	reply := make([]byte, 2)
	if _, err := io.ReadFull(conn, reply); err != nil {
		return err
	}
	if reply[0] != 0x05 || reply[1] != 0x00 {
		return fmt.Errorf("unexpected socks handshake reply: %v", reply)
	}
	return nil
}
