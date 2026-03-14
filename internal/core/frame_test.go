package core

import (
	"bytes"
	"testing"
)

func TestCodecRoundTrip(t *testing.T) {
	codec := Codec{}
	in := Frame{Type: FrameTCPDATA, SessionID: 7, ChannelID: 9, Seq: 11, Payload: []byte("hello")}
	var buf bytes.Buffer
	if err := codec.Encode(in, &buf); err != nil {
		t.Fatalf("encode: %v", err)
	}
	out, err := codec.Decode(&buf)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Type != in.Type || out.SessionID != in.SessionID || out.ChannelID != in.ChannelID || string(out.Payload) != string(in.Payload) {
		t.Fatalf("unexpected round trip: %#v", out)
	}
}

func TestCodecRejectsOversizedPayload(t *testing.T) {
	codec := Codec{}
	err := codec.Encode(Frame{Type: FrameTCPDATA, Payload: bytes.Repeat([]byte("a"), MaxPayloadLen+1)}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected oversized payload error")
	}
}
