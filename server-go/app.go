package main

import (
	"log"
	"net"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/zishang520/socket.io/v2/socket"
)

// App is the testable wiring of Gin + Socket.IO. main() and tests both use it.
type App struct {
	IO     *socket.Server
	Router *gin.Engine

	httpServer *http.Server
	listener   net.Listener
}

func NewApp() *App {
	opts := socket.DefaultServerOptions()
	// Default is 1 MB which truncates the large-payload tests; lift to 8 MB.
	opts.SetMaxHttpBufferSize(8 * 1024 * 1024)
	io := socket.NewServer(nil, opts)
	registerRootNamespace(io)
	registerAdminNamespace(io)

	r := gin.New()
	r.Use(gin.Recovery())
	if gin.Mode() != gin.TestMode {
		r.Use(gin.Logger())
	}

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true, "ts": time.Now().UnixMilli()})
	})

	sioHandler := io.ServeHandler(nil)
	r.GET("/socket.io/*any", gin.WrapH(sioHandler))
	r.POST("/socket.io/*any", gin.WrapH(sioHandler))

	return &App{IO: io, Router: r}
}

// Start listens on addr and serves in a goroutine. Returns the actual bound
// address (useful when addr uses :0).
func (a *App) Start(addr string) (string, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return "", err
	}
	a.listener = ln
	a.httpServer = &http.Server{Handler: a.Router}
	go func() {
		if err := a.httpServer.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Printf("serve error: %v", err)
		}
	}()
	return ln.Addr().String(), nil
}

func (a *App) Shutdown() error {
	a.IO.Close(nil)
	if a.httpServer != nil {
		return a.httpServer.Close()
	}
	return nil
}
