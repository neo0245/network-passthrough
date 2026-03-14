package auth

import (
	"testing"
	"time"
)

func TestReplayProtection(t *testing.T) {
	store, err := NewStore("")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	rec, err := store.Generate(time.Hour)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	var nonce [12]byte
	nonce[0] = 1
	if err := store.Validate(rec.Token, nonce); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if err := store.Validate(rec.Token, nonce); err == nil {
		t.Fatal("expected replay rejection")
	}
}
