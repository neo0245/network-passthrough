package core

import (
	"bytes"
	"testing"
)

func TestUDPFragmentReassembly(t *testing.T) {
	payload := bytes.Repeat([]byte("b"), MaxPayloadLen+100)
	frags, err := FragmentUDP(payload, 1, 1000)
	if err != nil {
		t.Fatalf("fragment: %v", err)
	}
	r := NewReassembler()
	var out []byte
	var ok bool
	for _, frag := range frags {
		out, ok, err = r.Add(frag)
		if err != nil {
			t.Fatalf("add: %v", err)
		}
	}
	if !ok {
		t.Fatal("expected reassembly")
	}
	if !bytes.Equal(out, payload) {
		t.Fatal("unexpected payload")
	}
}
