package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/zishang520/socket.io/v2/socket"
)

const addr = ":3000"

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	app := NewApp()
	bound, err := app.Start(addr)
	if err != nil {
		log.Fatalf("listen error: %v", err)
	}
	log.Printf("[server-go] Gin + Socket.IO listening on http://localhost%s", bound)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig
	log.Println("[server-go] shutting down...")
	_ = app.Shutdown()
}

func registerRootNamespace(io *socket.Server) {
	io.Use(jwtMiddleware("/"))

	io.On("connection", func(clients ...any) {
		client := clients[0].(*socket.Socket)
		sid := string(client.Id())
		claims, _ := client.Data().(*Claims)
		log.Printf("[/] connected: %s (uid=%s role=%s)", sid, claims.UserID, claims.Role)

		_ = client.Emit("welcome", map[string]any{
			"message": "hello from go-gin socket.io 4.x server",
			"id":      sid,
			"ts":      time.Now().UnixMilli(),
		})

		client.On("ping", func(args ...any) {
			var payload any
			ack, hasAck := extractAck(args)
			data := args
			if hasAck {
				data = args[:len(args)-1]
			}
			if len(data) > 0 {
				payload = data[0]
			}
			log.Printf("[/] ping from %s: %v", sid, summarize(payload))
			if hasAck {
				ack([]any{map[string]any{
					"pong": true,
					"echo": payload,
					"ts":   time.Now().UnixMilli(),
				}}, nil)
			}
		})

		client.On("chat", func(args ...any) {
			msg := firstArg(args)
			log.Printf("[/] chat from %s: %v", sid, summarize(msg))
			_ = io.Emit("chat", map[string]any{
				"from": sid,
				"msg":  msg,
				"ts":   time.Now().UnixMilli(),
			})
		})

		client.On("join", func(args ...any) {
			room := toString(firstArg(args))
			client.Join(socket.Room(room))
			log.Printf("[/] %s joined room %q", sid, room)
			_ = io.To(socket.Room(room)).Emit("system", sid+" joined "+room)
		})

		client.On("roomMsg", func(args ...any) {
			m, _ := firstArg(args).(map[string]any)
			if m == nil {
				return
			}
			room := toString(m["room"])
			_ = io.To(socket.Room(room)).Emit("roomMsg", map[string]any{
				"from": sid,
				"room": room,
				"msg":  m["msg"],
			})
		})

		client.On("disconnect", func(args ...any) {
			log.Printf("[/] disconnected %s: %v", sid, firstArg(args))
		})
	})
}

func registerAdminNamespace(io *socket.Server) {
	admin := io.Of("/admin", nil)
	admin.Use(jwtMiddleware("/admin"))
	admin.On("connection", func(clients ...any) {
		client := clients[0].(*socket.Socket)
		sid := string(client.Id())
		log.Printf("[/admin] connected: %s", sid)

		_ = client.Emit("welcome", map[string]any{
			"ns": "/admin",
			"id": sid,
		})

		client.On("op", func(args ...any) {
			ack, hasAck := extractAck(args)
			data := args
			if hasAck {
				data = args[:len(args)-1]
			}
			log.Printf("[/admin] op: %v", summarize(firstArg(data)))
			if hasAck {
				ack([]any{map[string]any{"ok": true}}, nil)
			}
		})
	})
}

func extractAck(args []any) (socket.Ack, bool) {
	if len(args) == 0 {
		return nil, false
	}
	ack, ok := args[len(args)-1].(socket.Ack)
	return ack, ok
}

func firstArg(args []any) any {
	if len(args) == 0 {
		return nil
	}
	return args[0]
}

func toString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// summarize avoids dumping huge payloads (e.g. 1MB strings) into logs during
// stress / large-payload tests; returns a short description for big values.
func summarize(v any) any {
	if s, ok := v.(string); ok && len(s) > 128 {
		return "<string len=" + itoa(len(s)) + ">"
	}
	return v
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b [20]byte
	pos := len(b)
	for i > 0 {
		pos--
		b[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(b[pos:])
}
