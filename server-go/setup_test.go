package main

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	clienttransports "github.com/zishang520/engine.io-client-go/transports"
	enginetypes "github.com/zishang520/engine.io/v2/types"
	clientsocket "github.com/zishang520/socket.io-client-go/socket"
)

var (
	testApp *App
	testURL string
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	testApp = NewApp()
	addr, err := testApp.Start("127.0.0.1:0")
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to start test server:", err)
		os.Exit(1)
	}
	testURL = "http://" + addr + "/"
	code := m.Run()
	_ = testApp.Shutdown()
	os.Exit(code)
}

// mustToken generates a valid HS256 token for tests using the same secret as
// the server (both default to "dev-secret-change-me" when JWT_SECRET unset).
func mustToken(t testing.TB, uid, role string, ttl time.Duration) string {
	t.Helper()
	tok, err := signTokenForDev(uid, role, ttl)
	if err != nil {
		t.Fatalf("signTokenForDev: %v", err)
	}
	return tok
}

// dial returns an unconnected socket. token == "" means no auth is sent.
func dial(t testing.TB, token string) *clientsocket.Socket {
	t.Helper()
	opts := clientsocket.DefaultOptions()
	opts.SetTransports(enginetypes.NewSet(clienttransports.Polling, clienttransports.WebSocket))
	if token != "" {
		opts.SetAuth(map[string]any{"token": token})
	}
	sock, err := clientsocket.Connect(testURL, opts)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() {
		sock.Disconnect()
		sock.Close()
	})
	return sock
}

// dialAndWait dials and blocks until either "connect" or "connect_error" fires.
// Returns nil if connect_error happened (test should not fail; caller decides).
func dialAndWait(t testing.TB, token string, timeout time.Duration) (*clientsocket.Socket, error) {
	t.Helper()
	sock := dial(t, token)
	connected := make(chan struct{})
	errCh := make(chan error, 1)
	var once sync.Once
	sock.On("connect", func(...any) { once.Do(func() { close(connected) }) })
	sock.On("connect_error", func(args ...any) {
		select {
		case errCh <- fmt.Errorf("connect_error: %v", args):
		default:
		}
	})
	select {
	case <-connected:
		return sock, nil
	case err := <-errCh:
		return nil, err
	case <-time.After(timeout):
		return nil, fmt.Errorf("timeout waiting for connect after %s", timeout)
	}
}

// mustDial is the success-path helper used by most tests.
func mustDial(t testing.TB, token string) *clientsocket.Socket {
	t.Helper()
	sock, err := dialAndWait(t, token, 5*time.Second)
	if err != nil {
		t.Fatalf("mustDial: %v", err)
	}
	return sock
}

// waitForConnect blocks until either "connect" or "connect_error" fires.
// Returns nil if connected, error from connect_error, or timeout error.
// Caller must register this BEFORE expecting connect to complete (use after dial*).
func waitForConnect(t testing.TB, sock *clientsocket.Socket, timeout time.Duration) error {
	t.Helper()
	connected := make(chan struct{})
	errCh := make(chan error, 1)
	var once sync.Once
	sock.On("connect", func(...any) { once.Do(func() { close(connected) }) })
	sock.On("connect_error", func(args ...any) {
		select {
		case errCh <- fmt.Errorf("connect_error: %v", args):
		default:
		}
	})
	select {
	case <-connected:
		return nil
	case err := <-errCh:
		return err
	case <-time.After(timeout):
		return fmt.Errorf("timeout waiting for connect after %s", timeout)
	}
}

// dialAdmin dials the /admin namespace with the given token in auth payload.
// Returns an unconnected socket; use waitForConnect on it.
func dialAdmin(t testing.TB, token string) *clientsocket.Socket {
	t.Helper()
	opts := clientsocket.DefaultOptions()
	opts.SetTransports(enginetypes.NewSet(clienttransports.Polling, clienttransports.WebSocket))
	if token != "" {
		opts.SetAuth(map[string]any{"token": token})
	}
	sock, err := clientsocket.Connect(testURL+"admin", opts)
	if err != nil {
		t.Fatalf("connect /admin: %v", err)
	}
	t.Cleanup(func() {
		sock.Disconnect()
		sock.Close()
	})
	return sock
}

// dialWithQuery sends the token via query string only (no auth payload, no header).
func dialWithQuery(t testing.TB, token string) *clientsocket.Socket {
	t.Helper()
	opts := clientsocket.DefaultOptions()
	opts.SetTransports(enginetypes.NewSet(clienttransports.Polling, clienttransports.WebSocket))
	if token != "" {
		q := url.Values{}
		q.Set("token", token)
		opts.SetQuery(q)
	}
	sock, err := clientsocket.Connect(testURL, opts)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() {
		sock.Disconnect()
		sock.Close()
	})
	return sock
}

// dialWithHeader sends a raw Authorization header (caller provides full value
// including "Bearer " prefix or variants for parser testing).
func dialWithHeader(t testing.TB, authorizationValue string) *clientsocket.Socket {
	t.Helper()
	opts := clientsocket.DefaultOptions()
	opts.SetTransports(enginetypes.NewSet(clienttransports.Polling, clienttransports.WebSocket))
	if authorizationValue != "" {
		h := http.Header{}
		h.Set("Authorization", authorizationValue)
		opts.SetExtraHeaders(h)
	}
	sock, err := clientsocket.Connect(testURL, opts)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() {
		sock.Disconnect()
		sock.Close()
	})
	return sock
}

// makeDualSourceOpts builds opts that may set token in any combination of
// auth payload, query string, and Authorization header. Empty string skips
// that source.
func makeDualSourceOpts(t testing.TB, authTok, queryTok, headerVal string) *clientsocket.Options {
	t.Helper()
	opts := clientsocket.DefaultOptions()
	opts.SetTransports(enginetypes.NewSet(clienttransports.Polling, clienttransports.WebSocket))
	if authTok != "" {
		opts.SetAuth(map[string]any{"token": authTok})
	}
	if queryTok != "" {
		q := url.Values{}
		q.Set("token", queryTok)
		opts.SetQuery(q)
	}
	if headerVal != "" {
		h := http.Header{}
		h.Set("Authorization", headerVal)
		opts.SetExtraHeaders(h)
	}
	return opts
}

// connectAndCleanup dials the given URL with preconfigured opts and registers
// disconnect cleanup. Used when the caller needs to set up unusual opts (e.g.
// multiple token sources for precedence tests).
func connectAndCleanup(t testing.TB, dialURL string, opts *clientsocket.Options) *clientsocket.Socket {
	t.Helper()
	sock, err := clientsocket.Connect(dialURL, opts)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() {
		sock.Disconnect()
		sock.Close()
	})
	return sock
}

// waitFor returns the first event payload or fails on timeout.
func waitFor(t testing.TB, sock *clientsocket.Socket, event string, timeout time.Duration) []any {
	t.Helper()
	ch := make(chan []any, 1)
	sock.Once(enginetypes.EventName(event), func(args ...any) {
		select {
		case ch <- args:
		default:
		}
	})
	select {
	case args := <-ch:
		return args
	case <-time.After(timeout):
		t.Fatalf("timeout waiting for event %q after %s", event, timeout)
		return nil
	}
}
