package core

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

type AuthPayload struct {
	Token string
	Nonce [12]byte
}

func EncodeAuthPayload(p AuthPayload) []byte {
	buf := bytes.NewBuffer(make([]byte, 0, 2+len(p.Token)+len(p.Nonce)))
	_ = binary.Write(buf, binary.BigEndian, uint16(len(p.Token)))
	buf.WriteString(p.Token)
	buf.Write(p.Nonce[:])
	return buf.Bytes()
}

func DecodeAuthPayload(b []byte) (AuthPayload, error) {
	var p AuthPayload
	if len(b) < 2+12 {
		return p, io.ErrUnexpectedEOF
	}
	tokenLen := int(binary.BigEndian.Uint16(b[:2]))
	if len(b) < 2+tokenLen+12 {
		return p, io.ErrUnexpectedEOF
	}
	p.Token = string(b[2 : 2+tokenLen])
	copy(p.Nonce[:], b[2+tokenLen:2+tokenLen+12])
	return p, nil
}

type OpenTCPPayload struct {
	Address string
}

func EncodeAddress(addr string) []byte {
	buf := bytes.NewBuffer(make([]byte, 0, 2+len(addr)))
	_ = binary.Write(buf, binary.BigEndian, uint16(len(addr)))
	buf.WriteString(addr)
	return buf.Bytes()
}

func DecodeAddress(b []byte) (string, error) {
	if len(b) < 2 {
		return "", io.ErrUnexpectedEOF
	}
	n := int(binary.BigEndian.Uint16(b[:2]))
	if len(b) < 2+n {
		return "", io.ErrUnexpectedEOF
	}
	return string(b[2 : 2+n]), nil
}

type UDPDatagram struct {
	ID         uint32
	SentMS     uint32
	DeadlineMS uint16
	FragIndex  uint16
	FragCount  uint16
	TotalLen   uint16
	Data       []byte
}

func EncodeUDPDatagram(d UDPDatagram) []byte {
	buf := bytes.NewBuffer(make([]byte, 0, 16+len(d.Data)))
	_ = binary.Write(buf, binary.BigEndian, d.ID)
	_ = binary.Write(buf, binary.BigEndian, d.SentMS)
	_ = binary.Write(buf, binary.BigEndian, d.DeadlineMS)
	_ = binary.Write(buf, binary.BigEndian, d.FragIndex)
	_ = binary.Write(buf, binary.BigEndian, d.FragCount)
	_ = binary.Write(buf, binary.BigEndian, d.TotalLen)
	buf.Write(d.Data)
	return buf.Bytes()
}

func DecodeUDPDatagram(b []byte) (UDPDatagram, error) {
	if len(b) < 16 {
		return UDPDatagram{}, io.ErrUnexpectedEOF
	}
	return UDPDatagram{
		ID:         binary.BigEndian.Uint32(b[:4]),
		SentMS:     binary.BigEndian.Uint32(b[4:8]),
		DeadlineMS: binary.BigEndian.Uint16(b[8:10]),
		FragIndex:  binary.BigEndian.Uint16(b[10:12]),
		FragCount:  binary.BigEndian.Uint16(b[12:14]),
		TotalLen:   binary.BigEndian.Uint16(b[14:16]),
		Data:       append([]byte(nil), b[16:]...),
	}, nil
}

type Reassembler struct {
	fragments map[uint32][][]byte
	expected  map[uint32]uint16
}

func NewReassembler() *Reassembler {
	return &Reassembler{
		fragments: make(map[uint32][][]byte),
		expected:  make(map[uint32]uint16),
	}
}

func (r *Reassembler) Add(d UDPDatagram) ([]byte, bool, error) {
	if d.FragCount == 0 {
		return nil, false, errors.New("invalid fragment count")
	}
	if d.FragCount == 1 {
		return d.Data, true, nil
	}
	if _, ok := r.fragments[d.ID]; !ok {
		r.fragments[d.ID] = make([][]byte, d.FragCount)
		r.expected[d.ID] = d.FragCount
	}
	if int(d.FragIndex) >= len(r.fragments[d.ID]) {
		return nil, false, errors.New("fragment index out of range")
	}
	r.fragments[d.ID][d.FragIndex] = append([]byte(nil), d.Data...)
	for _, frag := range r.fragments[d.ID] {
		if frag == nil {
			return nil, false, nil
		}
	}
	out := make([]byte, 0, d.TotalLen)
	for _, frag := range r.fragments[d.ID] {
		out = append(out, frag...)
	}
	delete(r.fragments, d.ID)
	delete(r.expected, d.ID)
	return out, true, nil
}

func FragmentUDP(data []byte, id uint32, deadlineMS uint16) ([]UDPDatagram, error) {
	if len(data) == 0 {
		return []UDPDatagram{{ID: id, FragCount: 1}}, nil
	}
	maxChunk := MaxPayloadLen - 16
	if maxChunk <= 0 {
		return nil, errors.New("invalid chunk size")
	}
	fragCount := (len(data) + maxChunk - 1) / maxChunk
	out := make([]UDPDatagram, 0, fragCount)
	for i := 0; i < fragCount; i++ {
		start := i * maxChunk
		end := start + maxChunk
		if end > len(data) {
			end = len(data)
		}
		out = append(out, UDPDatagram{
			ID:         id,
			DeadlineMS: deadlineMS,
			FragIndex:  uint16(i),
			FragCount:  uint16(fragCount),
			TotalLen:   uint16(len(data)),
			Data:       append([]byte(nil), data[start:end]...),
		})
	}
	return out, nil
}

func JoinUDPAddress(host string, port uint16) string {
	return fmt.Sprintf("%s:%d", host, port)
}
