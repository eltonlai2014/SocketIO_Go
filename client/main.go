package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	clienttransports "github.com/zishang520/engine.io-client-go/transports"
	"github.com/zishang520/engine.io/v2/types"
	"github.com/zishang520/socket.io-client-go/socket"
)

const serverURL = "http://localhost:3000/"

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	log.Printf("connecting to %s ...", serverURL)

	opts := socket.DefaultOptions()
	opts.SetTransports(types.NewSet(clienttransports.Polling, clienttransports.WebSocket))

	if token := os.Getenv("JWT_TOKEN"); token != "" {
		opts.SetAuth(map[string]any{"token": token})
		log.Printf("auth: sending JWT (len=%d)", len(token))
	} else {
		log.Printf("auth: no JWT_TOKEN set; expect connect_error if server enforces auth")
	}

	sock, err := socket.Connect(serverURL, opts)
	if err != nil {
		log.Fatalf("connect error: %v", err)
	}

	sock.On("connect", func(_ ...any) {
		log.Printf("connected, sid=%s", sock.Id())

		// Plain emit (no ack).
		sock.Emit("chat", "hello from go client")

		// Emit with ack — server side handler signature is (payload, ack).
		sock.EmitWithAck("ping", map[string]any{"hello": "world"})(func(args []any, err error) {
			if err != nil {
				log.Printf("ack ping error: %v", err)
				return
			}
			log.Printf("ack ping <- %v", args)
		})

		// Room demo.
		sock.Emit("join", "go-room")
		time.AfterFunc(500*time.Millisecond, func() {
			sock.Emit("roomMsg", map[string]any{
				"room": "go-room",
				"msg":  "hello room from go",
			})
		})
	})

	sock.On("welcome", func(args ...any) { log.Printf("welcome <- %v", args) })
	sock.On("chat", func(args ...any) { log.Printf("chat    <- %v", args) })
	sock.On("roomMsg", func(args ...any) { log.Printf("roomMsg <- %v", args) })
	sock.On("system", func(args ...any) { log.Printf("system  <- %v", args) })

	sock.On("disconnect", func(args ...any) { log.Printf("disconnected: %v", args) })
	sock.On("connect_error", func(args ...any) { log.Printf("connect_error: %v", args) })

	// Catch-all for any unhandled events.
	sock.OnAny(func(args ...any) {
		log.Printf("[any] %v", args)
	})

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig
	log.Println("shutting down...")
	sock.Disconnect()
	sock.Close()
}
