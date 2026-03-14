package transport

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"slpp/internal/core"
)

type ClientTunnel struct {
	WriteCloser io.WriteCloser
	ReadCloser  io.ReadCloser
	Response    *http.Response
}

type BrowserProfile struct {
	Name    string
	Headers map[string]string
}

func DialClientTunnel(ctx context.Context, cfg core.ClientConfig) (*ClientTunnel, error) {
	pr, pw := io.Pipe()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.ServerURL+"/tunnel", pr)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	profile, err := ResolveBrowserProfile(cfg.BrowserProfile)
	if err != nil {
		return nil, err
	}
	ApplyBrowserProfile(req, profile)
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

func ResolveBrowserProfile(name string) (BrowserProfile, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "chrome":
		return BrowserProfile{
			Name: "chrome",
			Headers: map[string]string{
				"User-Agent":                "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/133.0.0.0 Safari/537.36",
				"Accept":                    "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8",
				"Accept-Language":           "en-US,en;q=0.9",
				"Cache-Control":             "max-age=0",
				"Sec-Ch-Ua":                 "\"Chromium\";v=\"133\", \"Google Chrome\";v=\"133\", \"Not(A:Brand\";v=\"99\"",
				"Sec-Ch-Ua-Mobile":          "?0",
				"Sec-Ch-Ua-Platform":        "\"Windows\"",
				"Sec-Fetch-Dest":            "document",
				"Sec-Fetch-Mode":            "navigate",
				"Sec-Fetch-Site":            "none",
				"Sec-Fetch-User":            "?1",
				"Upgrade-Insecure-Requests": "1",
			},
		}, nil
	case "firefox":
		return BrowserProfile{
			Name: "firefox",
			Headers: map[string]string{
				"User-Agent":                "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:136.0) Gecko/20100101 Firefox/136.0",
				"Accept":                    "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8",
				"Accept-Language":           "en-US,en;q=0.5",
				"Cache-Control":             "max-age=0",
				"Sec-Fetch-Dest":            "document",
				"Sec-Fetch-Mode":            "navigate",
				"Sec-Fetch-Site":            "none",
				"Sec-Fetch-User":            "?1",
				"Upgrade-Insecure-Requests": "1",
			},
		}, nil
	case "safari":
		return BrowserProfile{
			Name: "safari",
			Headers: map[string]string{
				"User-Agent":      "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.3 Safari/605.1.15",
				"Accept":          "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
				"Accept-Language": "en-US,en;q=0.9",
				"Cache-Control":   "max-age=0",
			},
		}, nil
	default:
		return BrowserProfile{}, fmt.Errorf("unsupported browser profile %q", name)
	}
}

func ApplyBrowserProfile(req *http.Request, profile BrowserProfile) {
	for k, v := range profile.Headers {
		req.Header.Set(k, v)
	}
}
