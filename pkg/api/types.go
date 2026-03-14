package api

import "time"

type OutputEnvelope struct {
	OK      bool        `json:"ok"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

type ConnectRequest struct {
	ServerURL string `json:"server_url"`
	Token     string `json:"token"`
}

type DisconnectRequest struct {
	Reason string `json:"reason,omitempty"`
}

type Profile struct {
	Version   string `json:"version"`
	ServerURL string `json:"server_url"`
	ListenTCP string `json:"listen_tcp,omitempty"`
	ListenUDP string `json:"listen_udp,omitempty"`
}

type TokenStatus struct {
	ID        string    `json:"id"`
	Revoked   bool      `json:"revoked"`
	ExpiresAt time.Time `json:"expires_at"`
}

type StatsSnapshot struct {
	StartedAt          time.Time `json:"started_at"`
	ActiveSessions     int       `json:"active_sessions"`
	ActiveChannels     int       `json:"active_channels"`
	FramesIn           uint64    `json:"frames_in"`
	FramesOut          uint64    `json:"frames_out"`
	BytesIn            uint64    `json:"bytes_in"`
	BytesOut           uint64    `json:"bytes_out"`
	DroppedUDP         uint64    `json:"dropped_udp"`
	ExpiredUDP         uint64    `json:"expired_udp"`
	AuthFailures       uint64    `json:"auth_failures"`
	Reconnects         uint64    `json:"reconnects"`
	SessionQueueBytes  uint64    `json:"session_queue_bytes"`
	ChannelQueueBytes  uint64    `json:"channel_queue_bytes"`
}

type Event struct {
	Time    time.Time `json:"time"`
	Level   string    `json:"level"`
	Message string    `json:"message"`
}
