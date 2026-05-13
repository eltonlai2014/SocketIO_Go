package main

import (
	"strings"
	"testing"
	"time"
)

// --- /admin namespace (JWT-only; no role gating per CLAUDE.md) -------------

func TestAdmin_NoToken_Rejected(t *testing.T) {
	sock := dialAdmin(t, "")
	err := waitForConnect(t, sock, 3*time.Second)
	if err == nil {
		t.Fatal("expected connect_error on /admin without token, got success")
	}
	if !strings.Contains(err.Error(), "unauthorized") {
		t.Fatalf("expected unauthorized, got: %v", err)
	}
}

func TestAdmin_ValidToken_Accepted_AnyRole(t *testing.T) {
	// JWT-only policy: any role can enter /admin.
	tok := mustToken(t, "alice", "user", time.Hour)
	sock := dialAdmin(t, tok)
	if err := waitForConnect(t, sock, 3*time.Second); err != nil {
		t.Fatalf("expected /admin to accept any valid JWT, got: %v", err)
	}
}

func TestAdmin_Welcome_PayloadShape(t *testing.T) {
	tok := mustToken(t, "alice", "admin", time.Hour)
	sock := dialAdmin(t, tok)

	args := waitFor(t, sock, "welcome", 3*time.Second)
	if len(args) < 1 {
		t.Fatal("admin welcome missing payload")
	}
	m, ok := args[0].(map[string]any)
	if !ok {
		t.Fatalf("admin welcome payload is not a map: %T", args[0])
	}
	if m["ns"] != "/admin" {
		t.Errorf("admin welcome.ns: got %v, want /admin", m["ns"])
	}
	if _, ok := m["id"].(string); !ok {
		t.Errorf("admin welcome.id missing or wrong type: %v", m["id"])
	}
}

func TestAdmin_OpAck_RoundTrip(t *testing.T) {
	tok := mustToken(t, "alice", "admin", time.Hour)
	sock := dialAdmin(t, tok)
	if err := waitForConnect(t, sock, 3*time.Second); err != nil {
		t.Fatalf("connect /admin: %v", err)
	}

	type ackResult struct {
		args []any
		err  error
	}
	ch := make(chan ackResult, 1)
	sock.EmitWithAck("op", map[string]any{"cmd": "noop"})(func(args []any, err error) {
		ch <- ackResult{args, err}
	})

	select {
	case r := <-ch:
		if r.err != nil {
			t.Fatalf("op ack error: %v", r.err)
		}
		if len(r.args) < 1 {
			t.Fatal("op ack payload empty")
		}
		m, ok := r.args[0].(map[string]any)
		if !ok {
			t.Fatalf("op ack payload not a map: %T", r.args[0])
		}
		if m["ok"] != true {
			t.Errorf("op ack.ok != true: %v", m["ok"])
		}
	case <-time.After(3 * time.Second):
		t.Fatal("op ack timeout")
	}
}
