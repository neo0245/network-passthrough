package transport

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"net/http"
	"sync/atomic"
	"time"

	"slpp/internal/auth"
	"slpp/internal/core"
	"slpp/internal/relay"
)

type TunnelHandler struct {
	Validator *auth.Store
	Stats     *core.Stats
}

func (h *TunnelHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/tunnel", h.handleTunnel)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	})
}

func (h *TunnelHandler) handleTunnel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	handler := &serverSessionHandler{
		validator: h.Validator,
		stats:     h.Stats,
	}
	sessionID := uint32(time.Now().UnixNano())
	session := core.NewSession(sessionID, r.Body, &flushWriter{w: w, f: flusher}, handler, h.Stats, DefaultHeartbeat())
	handler.session = session
	handler.tcp = relay.NewTCPRelay(session)
	handler.udp = relay.NewUDPRelay(session)
	h.Stats.AddActiveSession(1)
	defer h.Stats.AddActiveSession(-1)
	_ = session.Run(r.Context())
}

type flushWriter struct {
	w http.ResponseWriter
	f http.Flusher
}

func (fw *flushWriter) Write(p []byte) (int, error) {
	n, err := fw.w.Write(p)
	fw.f.Flush()
	return n, err
}

type serverSessionHandler struct {
	session   *core.Session
	validator *auth.Store
	stats     *core.Stats
	tcp       *relay.TCPRelay
	udp       *relay.UDPRelay
	authOK    atomic.Bool
}

func (h *serverSessionHandler) HandleFrame(ctx context.Context, s *core.Session, frame core.Frame) error {
	if !h.authOK.Load() && frame.Type != core.FrameAUTH {
		return errors.New("auth required")
	}
	switch frame.Type {
	case core.FrameAUTH:
		authPayload, err := core.DecodeAuthPayload(frame.Payload)
		if err != nil {
			return err
		}
		if err := h.validator.Validate(authPayload.Token, authPayload.Nonce); err != nil {
			h.stats.AddAuthFailure()
			_ = s.QueueControl(core.Frame{Type: core.FrameAUTHERR, Payload: []byte(err.Error())})
			return err
		}
		h.authOK.Store(true)
		return s.QueueControl(core.Frame{Type: core.FrameAUTHOK})
	case core.FramePING:
		return s.QueueControl(core.Frame{Type: core.FramePONG})
	case core.FrameOPENTCP:
		addr, err := core.DecodeAddress(frame.Payload)
		if err != nil {
			return err
		}
		if err := h.tcp.Open(ctx, frame.ChannelID, addr); err != nil {
			return s.QueueControl(core.Frame{Type: core.FrameOPENERR, ChannelID: frame.ChannelID, Payload: []byte(err.Error())})
		}
		h.stats.AddActiveChannel(1)
		return s.QueueControl(core.Frame{Type: core.FrameOPENOK, ChannelID: frame.ChannelID})
	case core.FrameTCPDATA:
		return h.tcp.Write(frame.ChannelID, frame.Payload)
	case core.FrameTCPCLOSE:
		h.tcp.Close(frame.ChannelID)
		h.stats.AddActiveChannel(-1)
		return nil
	case core.FrameOPENUDP:
		addr, err := core.DecodeAddress(frame.Payload)
		if err != nil {
			return err
		}
		if err := h.udp.Open(ctx, frame.ChannelID, addr); err != nil {
			return s.QueueControl(core.Frame{Type: core.FrameOPENERR, ChannelID: frame.ChannelID, Payload: []byte(err.Error())})
		}
		h.stats.AddActiveChannel(1)
		return s.QueueControl(core.Frame{Type: core.FrameOPENOK, ChannelID: frame.ChannelID})
	case core.FrameUDPDATA:
		dg, err := core.DecodeUDPDatagram(frame.Payload)
		if err != nil {
			return err
		}
		if time.Since(time.UnixMilli(int64(dg.SentMS))) > time.Duration(dg.DeadlineMS)*time.Millisecond {
			h.stats.AddExpiredUDP()
			return nil
		}
		return h.udp.Write(frame.ChannelID, dg)
	case core.FrameUDPCLOSE:
		h.udp.Close(frame.ChannelID)
		h.stats.AddActiveChannel(-1)
		return nil
	case core.FrameSESSIONCLOSE:
		return context.Canceled
	default:
		return nil
	}
}

func NewHTTPServer(cfg core.ServerConfig, tunnelHandler *TunnelHandler) *http.Server {
	mux := http.NewServeMux()
	tunnelHandler.Register(mux)
	return &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		TLSConfig: &tls.Config{
			MinVersion: tls.VersionTLS13,
			NextProtos: []string{"h2", "http/1.1"},
		},
	}
}
