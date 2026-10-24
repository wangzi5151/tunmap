// Package mobile exposes the tunmap client as a gomobile-bindable library
// for Android (and iOS) apps.
package mobile

import (
	"log"
	"sync"

	"github.com/tunmap/tunmap/internal/client"
)

// Listener receives callbacks from the tunnel.
type Listener interface {
	OnReady(publicURL string)
	OnLog(line string)
	OnError(msg string)
}

var (
	mu      sync.Mutex
	stopCh  chan struct{}
	running bool
)

// Start dials the server and exposes localPort (a local port of THIS device).
// It returns immediately; progress is delivered through l.
func Start(server, key string, port int64, subdomain string, l Listener) {
	mu.Lock()
	if running {
		mu.Unlock()
		l.OnError("已经在运行了，先点停止")
		return
	}
	ch := make(chan struct{})
	stopCh = ch
	running = true
	mu.Unlock()

	go runLoop(server, key, port, subdomain, ch, l)
}

// Stop asks the tunnel goroutine to disconnect. Safe to call repeatedly.
func Stop() {
	mu.Lock()
	if stopCh != nil {
		close(stopCh)
		stopCh = nil
	}
	mu.Unlock()
}

func runLoop(server, key string, port int64, subdomain string, stop <-chan struct{}, l Listener) {
	defer func() {
		mu.Lock()
		running = false
		mu.Unlock()
	}()

	// capture client logs and forward them to the UI
	prevFlags := log.Flags()
	log.SetFlags(0)
	log.SetOutput(&listenerWriter{l: l})
	defer func() {
		log.SetFlags(prevFlags)
	}()

	client.Run(client.Options{
		Server:    server,
		Key:       key,
		LocalPort: int(port),
		Subdomain: subdomain,
	}, stop)

	l.OnLog("已停止")
}

type listenerWriter struct {
	l Listener
}

func (w *listenerWriter) Write(p []byte) (int, error) {
	w.l.OnLog(string(p))
	return len(p), nil
}
