package core

import (
	"crypto/tls"
	"time"
)

type ClientConfig struct {
	ServerURL       string
	Token           string
	ListenSocks     string
	UDPAssociate    string
	InsecureSkipTLS bool
	Heartbeat       time.Duration
}

type ServerConfig struct {
	ListenAddr      string
	CertFile        string
	KeyFile         string
	TokenFile       string
	ControlSocket   string
	Heartbeat       time.Duration
	IdleTimeout     time.Duration
	TLSConfig       *tls.Config
}
