package transport

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"io"
	"net/http"
	"time"

	"slpp/internal/core"
)

type ClientTunnel struct {
	WriteCloser io.WriteCloser
	ReadCloser  io.ReadCloser
	Response    *http.Response
}

func DialClientTunnel(ctx context.Context, cfg core.ClientConfig) (*ClientTunnel, error) {
	pr, pw := io.Pipe()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.ServerURL+"/tunnel", pr)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			MinVersion:         tls.VersionTLS13,
			InsecureSkipVerify: cfg.InsecureSkipTLS,
			NextProtos:         []string{"h2"},
		},
		ForceAttemptHTTP2: true,
	}
	client := &http.Client{Transport: transport}
	resp, err := client.Do(req)
	if err != nil {
		_ = pw.Close()
		return nil, err
	}
	return &ClientTunnel{WriteCloser: pw, ReadCloser: resp.Body, Response: resp}, nil
}

func NewAuthFrame(token string) (core.Frame, error) {
	var nonce [12]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return core.Frame{}, err
	}
	return core.Frame{
		Type:    core.FrameAUTH,
		Payload: core.EncodeAuthPayload(core.AuthPayload{Token: token, Nonce: nonce}),
	}, nil
}

func DefaultHeartbeat() time.Duration {
	return core.DefaultHeartbeatSecs * time.Second
}
