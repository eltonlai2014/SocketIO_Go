package main

import (
	"strings"
	"sync"
	"testing"
	"time"
)

// --- Auth gate -------------------------------------------------------------

func TestConnect_NoToken_Rejected(t *testing.T) {
	_, err := dialAndWait(t, "", 3*time.Second)
	if err == nil {
		t.Fatal("expected connect_error, got success")
	}
	if !strings.Contains(err.Error(), "unauthorized") {
		t.Fatalf("expected unauthorized error, got: %v", err)
	}
}

func TestConnect_ExpiredToken_Rejected(t *testing.T) {
	tok := mustToken(t, "bob", "user", -1*time.Minute)
	_, err := dialAndWait(t, tok, 3*time.Second)
	if err == nil {
		t.Fatal("expected connect_error, got success")
	}
	if !strings.Contains(err.Error(), "unauthorized") {
		t.Fatalf("expected unauthorized error, got: %v", err)
	}
}

func TestConnect_ValidToken_Accepted(t *testing.T) {
	tok := mustToken(t, "alice", "user", time.Hour)
	sock := mustDial(t, tok)
	if sock.Id() == "" {
		t.Fatal("expected non-empty sid")
	}
}

// --- Event correctness -----------------------------------------------------

func TestEvent_Welcome_PayloadShape(t *testing.T) {
	tok := mustToken(t, "alice", "user", time.Hour)
	sock := dial(t, tok)

	args := waitFor(t, sock, "welcome", 3*time.Second)
	if len(args) < 1 {
		t.Fatal("welcome missing payload")
	}
	m, ok := args[0].(map[string]any)
	if !ok {
		t.Fatalf("welcome payload is not a map: %T", args[0])
	}
	if s, _ := m["message"].(string); !strings.Contains(s, "go-gin") {
		t.Errorf("welcome.message unexpected: %v", m["message"])
	}
	if _, ok := m["id"].(string); !ok {
		t.Errorf("welcome.id missing or wrong type: %v", m["id"])
	}
}

func TestEvent_PingAck_RoundTrip(t *testing.T) {
	tok := mustToken(t, "alice", "user", time.Hour)
	sock := mustDial(t, tok)

	type ackResult struct {
		args []any
		err  error
	}
	ch := make(chan ackResult, 1)

	payload := map[string]any{"hello": "world", "n": float64(42)}
	sock.EmitWithAck("ping", payload)(func(args []any, err error) {
		ch <- ackResult{args, err}
	})

	select {
	case r := <-ch:
		if r.err != nil {
			t.Fatalf("ack error: %v", r.err)
		}
		if len(r.args) < 1 {
			t.Fatal("ack payload empty")
		}
		m, ok := r.args[0].(map[string]any)
		if !ok {
			t.Fatalf("ack payload not a map: %T", r.args[0])
		}
		if m["pong"] != true {
			t.Errorf("ack.pong != true: %v", m["pong"])
		}
		echo, _ := m["echo"].(map[string]any)
		if echo == nil || echo["hello"] != "world" || echo["n"] != float64(42) {
			t.Errorf("ack.echo did not round-trip: %v", m["echo"])
		}
	case <-time.After(3 * time.Second):
		t.Fatal("ack timeout")
	}
}

func TestEvent_ChatBroadcast_BetweenTwoClients(t *testing.T) {
	tokA := mustToken(t, "alice", "user", time.Hour)
	tokB := mustToken(t, "bob", "user", time.Hour)
	a := mustDial(t, tokA)
	b := mustDial(t, tokB)

	var wg sync.WaitGroup
	wg.Add(1)
	received := make(chan map[string]any, 1)
	b.On("chat", func(args ...any) {
		if len(args) == 0 {
			return
		}
		m, ok := args[0].(map[string]any)
		if !ok {
			return
		}
		// b also receives its own future messages — only count the one from a.
		if m["from"] == string(a.Id()) {
			select {
			case received <- m:
				wg.Done()
			default:
			}
		}
	})

	if err := a.Emit("chat", "hello from alice"); err != nil {
		t.Fatalf("emit: %v", err)
	}

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("b did not receive chat from a")
	}

	m := <-received
	if m["msg"] != "hello from alice" {
		t.Errorf("chat.msg mismatch: %v", m["msg"])
	}
}

func TestEvent_RoomScoped_OnlyMembersReceive(t *testing.T) {
	tok := func(uid string) string { return mustToken(t, uid, "user", time.Hour) }
	a := mustDial(t, tok("alice"))
	b := mustDial(t, tok("bob"))
	c := mustDial(t, tok("carol")) // not joining the room

	room := "room-" + time.Now().Format("150405.000000")

	bGot := make(chan map[string]any, 1)
	cGot := make(chan map[string]any, 1)
	b.On("roomMsg", func(args ...any) {
		if m, ok := args[0].(map[string]any); ok {
			select {
			case bGot <- m:
			default:
			}
		}
	})
	c.On("roomMsg", func(args ...any) {
		if m, ok := args[0].(map[string]any); ok {
			select {
			case cGot <- m:
			default:
			}
		}
	})

	// a and b join; c stays outside
	_ = a.Emit("join", room)
	_ = b.Emit("join", room)
	time.Sleep(200 * time.Millisecond) // let server-side Join() settle

	_ = a.Emit("roomMsg", map[string]any{"room": room, "msg": "only members"})

	select {
	case m := <-bGot:
		if m["msg"] != "only members" {
			t.Errorf("b got wrong msg: %v", m["msg"])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("b (member) did not receive roomMsg")
	}

	select {
	case m := <-cGot:
		t.Errorf("c (non-member) should NOT have received roomMsg: %v", m)
	case <-time.After(500 * time.Millisecond):
		// expected: silence
	}
}
