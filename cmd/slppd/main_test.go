package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"slpp/internal/auth"
	"slpp/internal/testutil"
)

func TestRunServerMainRequiresCertAndKey(t *testing.T) {
	t.Parallel()
	_, err := runServerMain(context.Background(), serverRunOptions{ListenAddr: "127.0.0.1:0"})
	if err == nil {
		t.Fatal("expected missing cert/key error")
	}
}

func TestCheckServerMain(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	tokenFile := filepath.Join(dir, "tokens.json")
	out, err := checkServerMain("127.0.0.1:8443", tokenFile)
	if err != nil {
		t.Fatalf("checkServerMain: %v", err)
	}
	if !out.OK {
		t.Fatalf("expected ok output: %#v", out)
	}

	invalidFile := filepath.Join(dir, "invalid.json")
	if err := os.WriteFile(invalidFile, []byte("{not-json"), 0o600); err != nil {
		t.Fatalf("write invalid file: %v", err)
	}
	if _, err := checkServerMain("127.0.0.1:8443", invalidFile); err == nil {
		t.Fatal("expected invalid token store error")
	}
}

func TestGenAndRevokeTokenMain(t *testing.T) {
	t.Parallel()
	tokenFile := filepath.Join(t.TempDir(), "tokens.json")

	out, err := genTokenMain(tokenFile, time.Hour)
	if err != nil {
		t.Fatalf("genTokenMain: %v", err)
	}
	raw, err := json.Marshal(out.Data)
	if err != nil {
		t.Fatalf("marshal token data: %v", err)
	}
	var rec auth.TokenRecord
	if err := json.Unmarshal(raw, &rec); err != nil {
		t.Fatalf("unmarshal token data: %v", err)
	}
	if rec.Token == "" || rec.ID == "" {
		t.Fatalf("invalid token record: %#v", rec)
	}

	if _, err := revokeTokenMain(tokenFile, rec.ID); err != nil {
		t.Fatalf("revokeTokenMain: %v", err)
	}

	store, err := auth.NewStore(tokenFile)
	if err != nil {
		t.Fatalf("load token store: %v", err)
	}
	var nonce [12]byte
	if err := store.Validate(rec.Token, nonce); err == nil {
		t.Fatal("expected revoked token to fail validation")
	}
}

func TestStatsMainReadsControlSocket(t *testing.T) {
	server := testutil.StartServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	out, err := statsMain(ctx, server.ControlSocket)
	if err != nil {
		t.Fatalf("statsMain: %v", err)
	}
	if !out.OK {
		t.Fatalf("expected ok output: %#v", out)
	}
}
