package core

import (
	"encoding/binary"
	"fmt"
	"io"
)

type Frame struct {
	Version   uint8
	Type      uint8
	Flags      uint8
	HeaderLen uint8
	SessionID uint32
	ChannelID uint32
	Seq       uint32
	Payload   []byte
}

type FrameCodec interface {
	Encode(Frame, io.Writer) error
	Decode(io.Reader) (Frame, error)
}

type Codec struct{}

func (Codec) Encode(frame Frame, w io.Writer) error {
	if frame.Version == 0 {
		frame.Version = Version
	}
	if frame.HeaderLen == 0 {
		frame.HeaderLen = HeaderLen
	}
	if frame.Version != Version {
		return ErrInvalidVersion
	}
	if !KnownFrameType(frame.Type) {
		return ErrUnknownType
	}
	if len(frame.Payload) > MaxPayloadLen {
		return ErrPayloadTooLarge
	}
	var header [HeaderLen]byte
	header[0] = frame.Version
	header[1] = frame.Type
	header[2] = frame.Flags
	header[3] = frame.HeaderLen
	binary.BigEndian.PutUint32(header[4:8], frame.SessionID)
	binary.BigEndian.PutUint32(header[8:12], frame.ChannelID)
	binary.BigEndian.PutUint32(header[12:16], frame.Seq)
	binary.BigEndian.PutUint32(header[16:20], uint32(len(frame.Payload)))
	if _, err := w.Write(header[:]); err != nil {
		return err
	}
	if len(frame.Payload) == 0 {
		return nil
	}
	_, err := w.Write(frame.Payload)
	return err
}

func (Codec) Decode(r io.Reader) (Frame, error) {
	var header [HeaderLen]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return Frame{}, err
	}
	frame := Frame{
		Version:   header[0],
		Type:      header[1],
		Flags:     header[2],
		HeaderLen: header[3],
		SessionID: binary.BigEndian.Uint32(header[4:8]),
		ChannelID: binary.BigEndian.Uint32(header[8:12]),
		Seq:       binary.BigEndian.Uint32(header[12:16]),
	}
	if frame.Version != Version {
		return Frame{}, ErrInvalidVersion
	}
	if frame.HeaderLen < HeaderLen {
		return Frame{}, ErrInvalidHeader
	}
	if !KnownFrameType(frame.Type) {
		return Frame{}, ErrUnknownType
	}
	payloadLen := binary.BigEndian.Uint32(header[16:20])
	if payloadLen > MaxPayloadLen {
		return Frame{}, ErrPayloadTooLarge
	}
	if payloadLen == 0 {
		return frame, nil
	}
	frame.Payload = make([]byte, payloadLen)
	if _, err := io.ReadFull(r, frame.Payload); err != nil {
		return Frame{}, fmt.Errorf("read payload: %w", err)
	}
	return frame, nil
}
