// Package client runs on the intranet machine: it dials the public server
// over WebSocket and forwards incoming tunnel requests to localhost.
package client

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/gorilla/websocket"
	"github.com/tunmap/tunmap/internal/protocol"
)

// Options configures the intranet client.
type Options struct {
	Server    string // e.g. "http://example.com:8080"
	Key       string // shared key from the server
	LocalPort int    // the intranet port to expose, e.g. 8080
	Subdomain string // optional custom subdomain
}

// Run dials the server and blocks until the connection dies.
// It keeps reconnecting every 3s. When stop is closed (non-nil), it returns.
func Run(opt Options, stop <-chan struct{}) {
	for {
		err := dial(opt, stop)
		if err != nil {
			log.Printf("connection error: %v — retrying in 3s", err)
		}
		if stopped(stop) {
			return
		}
		select {
		case <-stop:
			return
		case <-time.After(3 * time.Second):
		}
	}
}

func stopped(stop <-chan struct{}) bool {
	if stop == nil {
		return false
	}
	select {
	case <-stop:
		return true
	default:
		return false
	}
}

// dial opens a WebSocket to the server, requests a tunnel and then serves
// forwarded HTTP requests. It returns when the connection dies or stop is
// closed (closing the socket interrupts the blocking read).
func dial(opt Options, stop <-chan struct{}) error {
	u, err := url.Parse(opt.Server)
	if err != nil {
		return err
	}
	scheme := "ws"
	if u.Scheme == "https" {
		scheme = "wss"
	}
	wsURL := fmt.Sprintf("%s://%s/ws?key=%s", scheme, u.Host, url.QueryEscape(opt.Key))

	log.Printf("connecting to %s ...", u.Host)
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		return err
	}

	// request a tunnel
	if err := ws.WriteJSON(&protocol.Message{
		Type:      protocol.TypeTunnelCreate,
		LocalPort: opt.LocalPort,
		Subdomain: opt.Subdomain,
	}); err != nil {
		ws.Close()
		return err
	}

	ws.SetReadLimit(1 << 22)
	done := make(chan error, 1)
	go func() {
		defer ws.Close()
		for {
			var m protocol.Message
			if err := ws.ReadJSON(&m); err != nil {
				done <- err
				return
			}
			switch m.Type {
			case protocol.TypeTunnelCreated:
				log.Printf("==================================================")
				log.Printf("  tunnel ready")
				log.Printf("  public URL : %s", m.PublicURL)
				log.Printf("  forwards   -> localhost:%d", opt.LocalPort)
				log.Printf("==================================================")
			case protocol.TypeTunnelError:
				done <- fmt.Errorf("server rejected tunnel: %s", m.Message)
				return
			case protocol.TypeHTTPRequest:
				forwardToLocal(ws, &m, opt.LocalPort)
			case protocol.TypePing:
				_ = ws.WriteJSON(&protocol.Message{Type: protocol.TypePong})
			}
		}
	}()

	select {
	case <-stop:
		ws.Close() // interrupts ReadJSON, read goroutine exits
		return nil
	case err := <-done:
		return err
	}
}

// forwardToLocal executes the intranet HTTP request and sends the response back.
func forwardToLocal(ws *websocket.Conn, m *protocol.Message, port int) {
	rawBody, _ := base64.StdEncoding.DecodeString(m.Body)
	target := fmt.Sprintf("http://127.0.0.1:%d%s", port, m.Path)

	req, err := http.NewRequest(m.Method, target, bytes.NewReader(rawBody))
	if err != nil {
		sendResponse(ws, m.ID, http.StatusBadGateway, nil, []byte(err.Error()))
		return
	}
	for k, v := range m.Headers {
		req.Header.Set(k, v)
	}

	client := &http.Client{Timeout: 25 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		sendResponse(ws, m.ID, http.StatusBadGateway, nil, []byte("localhost unreachable: "+err.Error()))
		return
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	headers := map[string]string{}
	for k, vals := range resp.Header {
		if len(vals) > 0 {
			headers[k] = vals[0]
		}
	}
	sendResponse(ws, m.ID, resp.StatusCode, headers, respBody)
}

func sendResponse(ws *websocket.Conn, id string, status int, headers map[string]string, body []byte) {
	msg := &protocol.Message{
		Type:    protocol.TypeHTTPResponse,
		ID:      id,
		Status:  status,
		Headers: headers,
		Body:    base64.StdEncoding.EncodeToString(body),
	}
	if err := ws.WriteJSON(msg); err != nil {
		log.Printf("write response failed: %v", err)
	}
}
