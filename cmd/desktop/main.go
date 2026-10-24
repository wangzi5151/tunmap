// Desktop client: double-click to run, opens the browser with a minimal
// control page (fill port -> start -> copy public URL -> stop).
package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/tunmap/tunmap/internal/client"
)

//go:embed web.html
var webUI []byte

// hub captures client log lines and broadcasts them over SSE.
type hub struct {
	mu      sync.Mutex
	lines   []string
	watcher chan string
}

func (h *hub) Write(p []byte) (int, error) {
	line := strings.TrimSpace(string(p))
	if line == "" {
		return len(p), nil
	}
	h.mu.Lock()
	h.lines = append(h.lines, line)
	if len(h.lines) > 100 {
		h.lines = h.lines[len(h.lines)-100:]
	}
	w := h.watcher
	h.mu.Unlock()
	if w != nil {
		select {
		case w <- line:
		default:
		}
	}
	return len(p), nil
}

// app holds tunnel lifecycle state.
type app struct {
	hub *hub

	mu        sync.Mutex
	running   bool
	stopCh    chan struct{}
	server    string
	port      int
	publicURL string
}

func (a *app) start(server string, port int) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.running {
		return fmt.Errorf("已在运行，请先点停止")
	}
	ch := make(chan struct{})
	a.stopCh = ch
	a.running = true
	a.server = server
	a.port = port
	a.publicURL = ""

	prev := log.Writer()
	log.SetOutput(a.hub)
	log.SetFlags(0)
	go func() {
		defer func() { log.SetOutput(prev) }()
		client.Run(client.Options{
			Server:    server,
			LocalPort: port,
		}, ch)
		a.mu.Lock()
		a.running = false
		a.mu.Unlock()
	}()
	return nil
}

func (a *app) stop() {
	a.mu.Lock()
	if a.stopCh != nil {
		close(a.stopCh)
		a.stopCh = nil
	}
	a.mu.Unlock()
}

func (a *app) status() map[string]any {
	a.mu.Lock()
	defer a.mu.Unlock()
	return map[string]any{
		"running":    a.running,
		"server":     a.server,
		"port":       a.port,
		"public_url": a.publicURL,
	}
}

func main() {
	a := &app{hub: &hub{}}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(webUI)
	})
	mux.HandleFunc("/api/start", func(w http.ResponseWriter, r *http.Request) {
		srv := r.URL.Query().Get("server")
		var p int
		fmt.Sscanf(r.URL.Query().Get("port"), "%d", &p)
		if srv == "" || p < 1 || p > 65535 {
			http.Error(w, "server 和 port 必填", http.StatusBadRequest)
			return
		}
		if err := a.start(srv, p); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(200)
	})
	mux.HandleFunc("/api/stop", func(w http.ResponseWriter, r *http.Request) {
		a.stop()
		w.WriteHeader(200)
	})
	mux.HandleFunc("/api/quit", func(w http.ResponseWriter, r *http.Request) {
		a.stop()
		w.WriteHeader(200)
		go func() {
			time.Sleep(300 * time.Millisecond)
			os.Exit(0)
		}()
	})
	mux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(a.status())
	})
	mux.HandleFunc("/api/events", func(w http.ResponseWriter, r *http.Request) {
		flusher, _ := w.(http.Flusher)
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("X-Accel-Buffering", "no")

		ch := make(chan string, 64)
		a.hub.mu.Lock()
		for _, l := range a.hub.lines {
			fmt.Fprintf(w, "event: log\ndata: %s\n\n", mustJSON(l))
		}
		a.hub.watcher = ch
		a.hub.mu.Unlock()
		flusher.Flush()

		defer func() {
			a.hub.mu.Lock()
			a.hub.watcher = nil
			a.hub.mu.Unlock()
		}()

		for {
			select {
			case <-r.Context().Done():
				return
			case l := <-ch:
				fmt.Fprintf(w, "event: log\ndata: %s\n\n", mustJSON(l))
				flusher.Flush()
				if strings.Contains(l, "public URL :") {
					url := strings.TrimSpace(strings.TrimPrefix(l, "public URL :"))
					a.mu.Lock()
					a.publicURL = url
					a.mu.Unlock()
				}
			}
		}
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatalf("listen: %v", err)
	}
	url := fmt.Sprintf("http://127.0.0.1:%d/", ln.Addr().(*net.TCPAddr).Port)
	go openBrowser(url)

	fmt.Println("tunmap desktop: " + url)
	if err := http.Serve(ln, mux); err != nil {
		log.Fatal(err)
	}
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return `""`
	}
	return string(b)
}
