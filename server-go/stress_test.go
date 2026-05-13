package main

import (
	"flag"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	clientsocket "github.com/zishang520/socket.io-client-go/socket"
)

// -stress N: number of concurrent clients to spin up.
//
//	go test -run TestStress -stress 200 -timeout 5m
var stressN = flag.Int("stress", 50, "concurrent clients for TestStress_*")

// TestStress_ManyConcurrentConnects verifies the server accepts N clients
// connecting simultaneously and each receives its "welcome".
func TestStress_ManyConcurrentConnects(t *testing.T) {
	n := *stressN
	tok := mustToken(t, "stress", "user", time.Hour)

	var welcomes int64
	var wg sync.WaitGroup
	wg.Add(n)

	deadline := time.Now().Add(30 * time.Second)
	start := time.Now()

	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			// Register the welcome listener BEFORE the handshake completes,
			// otherwise the welcome packet can race past our registration.
			sock := dial(t, tok)
			gotWelcome := make(chan struct{}, 1)
			sock.On("welcome", func(...any) {
				atomic.AddInt64(&welcomes, 1)
				select {
				case gotWelcome <- struct{}{}:
				default:
				}
			})
			select {
			case <-gotWelcome:
			case <-time.After(time.Until(deadline)):
				t.Errorf("client %d: welcome not received", i)
			}
		}(i)
	}

	wg.Wait()
	elapsed := time.Since(start)
	t.Logf("connected %d clients in %s (%.1f conns/sec)",
		n, elapsed, float64(n)/elapsed.Seconds())

	if got := atomic.LoadInt64(&welcomes); int(got) != n {
		t.Errorf("welcomes received = %d, want %d", got, n)
	}
}

// TestStress_BroadcastFanout: one client emits chat, all N clients (including
// sender) should receive it.
func TestStress_BroadcastFanout(t *testing.T) {
	const n = 20
	const marker = "ping-of-truth"
	tok := mustToken(t, "fanout", "user", time.Hour)

	clients := make([]*clientsocket.Socket, n)
	for i := 0; i < n; i++ {
		clients[i] = mustDial(t, tok)
	}

	var got int64
	var wg sync.WaitGroup
	wg.Add(n)
	for _, c := range clients {
		c := c
		c.On("chat", func(args ...any) {
			if len(args) == 0 {
				return
			}
			m, _ := args[0].(map[string]any)
			if m == nil || m["msg"] != marker {
				return
			}
			if atomic.AddInt64(&got, 1) <= int64(n) {
				wg.Done()
			}
		})
	}

	if err := clients[0].Emit("chat", marker); err != nil {
		t.Fatalf("emit: %v", err)
	}

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatalf("only %d / %d clients received broadcast", atomic.LoadInt64(&got), n)
	}
}
