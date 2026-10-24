package server

import (
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/tunmap/tunmap/internal/protocol"
)

// handleWebSocket upgrades an intranet client connection.
// If the server was started with --key, the client must present it: /ws?key=xxx
// If no key was configured, the server is open.
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	if s.cfg.AuthKey != "" {
		if r.URL.Query().Get("key") != s.cfg.AuthKey {
			http.Error(w, "invalid key", http.StatusUnauthorized)
			return
		}
	}
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	cc := &clientConn{server: s, ws: ws}
	go cc.writePump()
	cc.readPump()
}

func (cc *clientConn) writePump() {
	tk := time.NewTicker(25 * time.Second)
	defer tk.Stop()
	for range tk.C {
		if err := cc.send(&protocol.Message{Type: protocol.TypePing}); err != nil {
			return
		}
	}
}

func (cc *clientConn) readPump() {
	defer func() {
		cc.ws.Close()
		if cc.subdomain != "" {
			cc.server.hub.Delete(cc.subdomain)
			log.Printf("tunnel offline: %s -> localhost:%d", cc.subdomain, cc.localPort)
		}
	}()

	cc.ws.SetReadLimit(1 << 22)
	_ = cc.ws.SetReadDeadline(time.Now().Add(60 * time.Second))
	cc.ws.SetPongHandler(func(string) error {
		_ = cc.ws.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		var m protocol.Message
		if err := cc.ws.ReadJSON(&m); err != nil {
			break
		}
		switch m.Type {
		case protocol.TypeTunnelCreate:
			cc.onTunnelCreate(&m)
		case protocol.TypeHTTPResponse:
			if ch, ok := cc.pending.LoadAndDelete(m.ID); ok {
				ch.(chan *protocol.Message) <- &m
			}
		case protocol.TypePing:
			_ = cc.send(&protocol.Message{Type: protocol.TypePong})
		}
	}
}

func (cc *clientConn) onTunnelCreate(m *protocol.Message) {
	sub := m.Subdomain
	if sub == "" {
		sub = randomSubdomain()
	}
	publicURL := "http://" + sub + "." + cc.server.cfg.BaseDomain

	cc.subdomain = sub
	cc.localPort = m.LocalPort
	cc.server.hub.Store(sub, cc)
	log.Printf("tunnel active: %s -> localhost:%d", sub, m.LocalPort)
	_ = cc.send(&protocol.Message{
		Type:      protocol.TypeTunnelCreated,
		Subdomain: sub,
		PublicURL: publicURL,
	})
}

// serveTunnel reverse-proxies a public HTTP request to the intranet client.
func (s *Server) serveTunnel(w http.ResponseWriter, r *http.Request, sub string) {
	start := time.Now()
	status := http.StatusBadGateway

	v, ok := s.hub.Load(sub)
	if !ok {
		http.Error(w, `{"error":"tunnel offline"}`, http.StatusBadGateway)
		s.recordRequest(logEntry{Subdomain: sub, Method: r.Method, Path: r.URL.RequestURI(), Status: status, DurationMs: time.Since(start).Milliseconds(), At: start})
		return
	}
	cc := v.(*clientConn)

	body, _ := io.ReadAll(r.Body)
	reqBytes := int64(len(body))
	reqID := randomToken(8)

	headers := map[string]string{}
	for k, vals := range r.Header {
		if len(vals) > 0 {
			headers[k] = vals[0]
		}
	}
	reqBody := truncateBody(body)

	ch := make(chan *protocol.Message, 1)
	cc.pending.Store(reqID, ch)
	defer cc.pending.Delete(reqID)

	err := cc.send(&protocol.Message{
		Type: protocol.TypeHTTPRequest, ID: reqID,
		Method: r.Method, Path: r.URL.RequestURI(),
		Headers: headers, Body: base64.StdEncoding.EncodeToString(body),
	})
	if err != nil {
		http.Error(w, "tunnel send error", http.StatusBadGateway)
		s.recordRequest(logEntry{Subdomain: sub, Method: r.Method, Path: r.URL.RequestURI(), Status: status, DurationMs: time.Since(start).Milliseconds(), At: start, ReqHeaders: headers, ReqBody: reqBody, ReqBytes: reqBytes, LocalPort: cc.localPort})
		return
	}

	var respHeaders map[string]string
	var respBody string
	var respBytes int64

	select {
	case resp := <-ch:
		for k, vv := range resp.Headers {
			w.Header().Set(k, vv)
		}
		if resp.Status == 0 {
			resp.Status = 200
		}
		status = resp.Status
		respHeaders = resp.Headers
		w.WriteHeader(status)
		if resp.Body != "" {
			raw, _ := base64.StdEncoding.DecodeString(resp.Body)
			respBytes = int64(len(raw))
			respBody = truncateBody(raw)
			_, _ = w.Write(raw)
		}
	case <-time.After(requestTimeout):
		status = http.StatusGatewayTimeout
		http.Error(w, "tunnel timeout", http.StatusGatewayTimeout)
	}

	s.recordRequest(logEntry{
		Subdomain: sub, Method: r.Method, Path: r.URL.RequestURI(),
		Status: status, DurationMs: time.Since(start).Milliseconds(), At: start,
		ReqHeaders: headers, ReqBody: reqBody, ReqBytes: reqBytes,
		RespHeaders: respHeaders, RespBody: respBody, RespBytes: respBytes,
		LocalPort: cc.localPort,
	})
}

func truncateBody(raw []byte) string {
	const max = 4096
	if len(raw) > max {
		return string(raw[:max]) + fmt.Sprintf("\n... [truncated %d bytes]", len(raw)-max)
	}
	return string(raw)
}
