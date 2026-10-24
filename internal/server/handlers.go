package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
)

// ListenAndServe starts the HTTP server.
func (s *Server) ListenAndServe() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/tunnels", s.handleOnlineTunnels)
	mux.HandleFunc("/api/stats", s.handleStats)
	mux.HandleFunc("/api/events", s.handleEvents)
	mux.HandleFunc("/ws", s.handleWebSocket)
	mux.HandleFunc("/", s.dispatch)

	log.Printf("tunmap server listening on %s (base domain: %s)", s.cfg.Addr, s.cfg.BaseDomain)
	log.Printf("client command: tunmap-client --server http://your-domain%s --key %s --port 5050",
		s.cfg.Addr, s.cfg.AuthKey)
	return http.ListenAndServe(s.cfg.Addr, mux)
}

// dispatch routes by Host:
//   - exact base domain (or bare IP / localhost) -> web console
//   - "<sub>.<base>"                            -> tunnel reverse proxy
func (s *Server) dispatch(w http.ResponseWriter, r *http.Request) {
	host := strings.Split(r.Host, ":")[0]
	if host == s.cfg.BaseDomain || host == "localhost" || isIP(host) {
		s.serveConsole(w, r)
		return
	}
	suffix := "." + s.cfg.BaseDomain
	if !strings.HasSuffix(host, suffix) {
		http.Error(w, "unknown host: "+host, http.StatusBadGateway)
		return
	}
	sub := strings.TrimSuffix(host, suffix)
	s.serveTunnel(w, r, sub)
}

func isIP(h string) bool {
	return strings.Count(h, ".") == 3 && !strings.ContainsAny(h, "abcdefghijklmnopqrstuvwxyz")
}

func (s *Server) serveConsole(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(embeddedWeb)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// handleOnlineTunnels returns the list of currently connected tunnels.
func (s *Server) handleOnlineTunnels(w http.ResponseWriter, r *http.Request) {
	var out []OnlineTunnel
	s.hub.Range(func(key, value any) bool {
		cc := value.(*clientConn)
		out = append(out, OnlineTunnel{
			Subdomain: cc.subdomain,
			LocalPort: cc.localPort,
			PublicURL: "http://" + cc.subdomain + "." + s.cfg.BaseDomain,
		})
		return true
	})
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	s.statsMu.Lock()
	var totalReq, reqBytes, respBytes int64
	per := map[string]tunnelStat{}
	for k, v := range s.tunStats {
		totalReq += v.Requests
		reqBytes += v.ReqBytes
		respBytes += v.RespBytes
		per[k] = *v
	}
	var online []OnlineTunnel
	s.hub.Range(func(key, value any) bool {
		cc := value.(*clientConn)
		online = append(online, OnlineTunnel{
			Subdomain: cc.subdomain, LocalPort: cc.localPort,
			PublicURL: "http://" + cc.subdomain + "." + s.cfg.BaseDomain,
		})
		return true
	})
	logs := make([]logEntry, len(s.logs))
	copy(logs, s.logs)
	s.statsMu.Unlock()

	for i, j := 0, len(logs)-1; i < j; i, j = i+1, j-1 {
		logs[i], logs[j] = logs[j], logs[i]
	}
	writeJSON(w, http.StatusOK, StatsResponse{
		OnlineTunnels: online, TotalRequests: totalReq,
		ReqBytes: reqBytes, RespBytes: respBytes,
		RecentLogs: logs, PerTunnel: per, BaseDomain: s.cfg.BaseDomain,
	})
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	ch := make(chan logEntry, 32)
	s.statsMu.Lock()
	s.watchers[ch] = struct{}{}
	recent := make([]logEntry, len(s.logs))
	copy(recent, s.logs)
	s.statsMu.Unlock()

	defer func() {
		s.statsMu.Lock()
		delete(s.watchers, ch)
		s.statsMu.Unlock()
	}()

	for i := len(recent) - 1; i >= 0; i-- {
		writeSSE(w, "log", recent[i])
	}
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case e := <-ch:
			writeSSE(w, "log", e)
			flusher.Flush()
		}
	}
}

func writeSSE(w http.ResponseWriter, event string, v any) {
	b, _ := json.Marshal(v)
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, b)
}
