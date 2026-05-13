package main

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestPayload_LargeAckRoundTrip verifies a multi-MB payload survives a ping/ack
// round-trip unchanged. Uses sha256 to detect any corruption.
func TestPayload_LargeAckRoundTrip(t *testing.T) {
	tok := mustToken(t, "alice", "user", time.Hour)
	sock := mustDial(t, tok)

	const size = 1 << 20 // 1 MiB
	body := strings.Repeat("ABCD0123", size/8)
	if len(body) != size {
		t.Fatalf("body size mismatch: %d", len(body))
	}
	wantHash := sha256.Sum256([]byte(body))

	type ackResult struct {
		args []any
		err  error
	}
	ch := make(chan ackResult, 1)

	start := time.Now()
	sock.EmitWithAck("ping", body)(func(args []any, err error) {
		ch <- ackResult{args, err}
	})

	select {
	case r := <-ch:
		if r.err != nil {
			t.Fatalf("ack error: %v", r.err)
		}
		m, ok := r.args[0].(map[string]any)
		if !ok {
			t.Fatalf("ack payload not a map: %T", r.args[0])
		}
		got, ok := m["echo"].(string)
		if !ok {
			t.Fatalf("ack.echo not a string: %T", m["echo"])
		}
		if len(got) != size {
			t.Fatalf("echo length mismatch: got %d want %d", len(got), size)
		}
		gotHash := sha256.Sum256([]byte(got))
		if gotHash != wantHash {
			t.Fatalf("hash mismatch:\n  want %s\n  got  %s",
				hex.EncodeToString(wantHash[:]), hex.EncodeToString(gotHash[:]))
		}
		t.Logf("round-tripped %d bytes in %s (%.1f MB/s)",
			size, time.Since(start),
			float64(size*2)/time.Since(start).Seconds()/(1024*1024))
	case <-time.After(15 * time.Second):
		t.Fatal("large ack timeout")
	}
}

// TestPayload_BurstManyMessages emits N back-to-back pings without waiting and
// verifies all acks come back. Tests pipelining / queue capacity.
func TestPayload_BurstManyMessages(t *testing.T) {
	const n = 500
	tok := mustToken(t, "alice", "user", time.Hour)
	sock := mustDial(t, tok)

	var got int64
	var wg sync.WaitGroup
	wg.Add(n)

	mismatches := make(chan string, n)

	start := time.Now()
	for i := 0; i < n; i++ {
		i := i
		sock.EmitWithAck("ping", map[string]any{"seq": float64(i)})(func(args []any, err error) {
			defer wg.Done()
			if err != nil {
				mismatches <- "ack err: " + err.Error()
				return
			}
			m, _ := args[0].(map[string]any)
			echo, _ := m["echo"].(map[string]any)
			if echo == nil || echo["seq"] != float64(i) {
				mismatches <- "seq mismatch"
				return
			}
			atomic.AddInt64(&got, 1)
		})
	}

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
		close(mismatches)
	case <-time.After(30 * time.Second):
		t.Fatalf("only %d / %d acks received before timeout", atomic.LoadInt64(&got), n)
	}

	for m := range mismatches {
		t.Errorf("burst error: %s", m)
	}

	if int(atomic.LoadInt64(&got)) != n {
		t.Fatalf("acks = %d, want %d", got, n)
	}
	elapsed := time.Since(start)
	t.Logf("%d round-trips in %s (%.0f msg/s)", n, elapsed, float64(n)/elapsed.Seconds())
}

// TestPayload_ConcurrentLargePayloads: K clients each push a large payload
// simultaneously, verifying isolated round-trip integrity per client.
func TestPayload_ConcurrentLargePayloads(t *testing.T) {
	const (
		clients = 5
		size    = 256 * 1024 // 256 KiB per client
	)
	tok := mustToken(t, "alice", "user", time.Hour)

	socks := make([]*returnedSock, clients)
	for i := range socks {
		socks[i] = &returnedSock{sock: mustDial(t, tok), id: i}
	}

	var wg sync.WaitGroup
	wg.Add(clients)
	errs := make(chan error, clients)

	start := time.Now()
	for _, s := range socks {
		s := s
		body := strings.Repeat(string('A'+byte(s.id)), size)
		hash := sha256.Sum256([]byte(body))
		go func() {
			defer wg.Done()
			done := make(chan error, 1)
			s.sock.EmitWithAck("ping", body)(func(args []any, err error) {
				if err != nil {
					done <- err
					return
				}
				m, _ := args[0].(map[string]any)
				echo, _ := m["echo"].(string)
				if len(echo) != size {
					done <- &payloadErr{cid: s.id, msg: "len mismatch"}
					return
				}
				if sha256.Sum256([]byte(echo)) != hash {
					done <- &payloadErr{cid: s.id, msg: "hash mismatch"}
					return
				}
				done <- nil
			})
			select {
			case err := <-done:
				if err != nil {
					errs <- err
				}
			case <-time.After(20 * time.Second):
				errs <- &payloadErr{cid: s.id, msg: "timeout"}
			}
		}()
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("client error: %v", err)
	}

	totalBytes := clients * size * 2
	t.Logf("%d clients × %d B round-tripped in %s (%.1f MB/s aggregate)",
		clients, size, time.Since(start),
		float64(totalBytes)/time.Since(start).Seconds()/(1024*1024))
}

type returnedSock struct {
	sock interface {
		EmitWithAck(string, ...any) func(func([]any, error))
	}
	id int
}

type payloadErr struct {
	cid int
	msg string
}

func (e *payloadErr) Error() string {
	return "client " + itoa(e.cid) + ": " + e.msg
}
