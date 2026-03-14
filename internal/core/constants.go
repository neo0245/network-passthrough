package core

import "errors"

const (
	Version                = 1
	HeaderLen              = 20
	MaxPayloadLen          = 16 * 1024
	MaxChannelsPerSession  = 128
	MaxQueuedBytesChannel  = 64 * 1024
	MaxQueuedBytesSession  = 8 * 1024 * 1024
	MaxLocalUDPDatagram    = 1200
	DefaultHeartbeatSecs   = 15
	DefaultIdleTimeoutSecs = 60
)

const (
	FrameAUTH         uint8 = 0x01
	FrameAUTHOK       uint8 = 0x02
	FrameAUTHERR      uint8 = 0x03
	FrameSETTINGS     uint8 = 0x10
	FramePING         uint8 = 0x11
	FramePONG         uint8 = 0x12
	FrameOPENTCP      uint8 = 0x20
	FrameOPENOK       uint8 = 0x21
	FrameOPENERR      uint8 = 0x22
	FrameTCPDATA      uint8 = 0x23
	FrameTCPCLOSE     uint8 = 0x24
	FrameOPENUDP      uint8 = 0x30
	FrameUDPDATA      uint8 = 0x31
	FrameUDPCLOSE     uint8 = 0x32
	FrameERROR        uint8 = 0x7E
	FrameSESSIONCLOSE uint8 = 0x7F
)

var (
	ErrInvalidVersion  = errors.New("invalid frame version")
	ErrInvalidHeader   = errors.New("invalid frame header")
	ErrPayloadTooLarge = errors.New("payload too large")
	ErrUnknownType     = errors.New("unknown frame type")
	ErrQueueFull       = errors.New("queue full")
)

func KnownFrameType(t uint8) bool {
	switch t {
	case FrameAUTH, FrameAUTHOK, FrameAUTHERR, FrameSETTINGS, FramePING, FramePONG,
		FrameOPENTCP, FrameOPENOK, FrameOPENERR, FrameTCPDATA, FrameTCPCLOSE,
		FrameOPENUDP, FrameUDPDATA, FrameUDPCLOSE, FrameERROR, FrameSESSIONCLOSE:
		return true
	default:
		return false
	}
}
