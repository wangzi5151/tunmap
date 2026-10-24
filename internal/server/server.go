// Package server is the public-side component: it terminates HTTP traffic
// from the internet, authenticates clients with a single shared key, and
// relays requests over a WebSocket tunnel to the intranet client.
package server

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/tunmap/tunmap/internal/protocol"
)

// Config holds server runtime configuration.
type Config struct {
	Addr       string // listen address, e.g. ":8080"
	BaseDomain string // e.g. "example.com" — subdomains like abc.example.com are tunneled
	AuthKey    string // shared secret; clients must present this key
}

// Server is the public-side tunnel controller.
type Server struct {
	cfg Config

	// hub: subdomain -> live client connection
	hub sync.Map

	// stats & live log broadcast
	statsMu  sync.Mutex
	tunStats map[string]*tunnelStat
	logs     []logEntry
	watchers map[chan logEntry]struct{}
}

type tunnelStat struct {
	Requests  int64
	ReqBytes  int64
	RespBytes int64
	LastSeen  time.Time
}

type logEntry struct {
	Subdomain   string            `json:"subdomain"`
	Method      string            `json:"method"`
	Path        string            `json:"path"`
	Status      int               `json:"status"`
	DurationMs  int64             `json:"duration_ms"`
	At          time.Time         `json:"at"`
	ReqHeaders  map[string]string `json:"req_headers,omitempty"`
	ReqBody     string            `json:"req_body,omitempty"`
	RespHeaders map[string]string `json:"resp_headers,omitempty"`
	RespBody    string            `json:"resp_body,omitempty"`
	ReqBytes    int64             `json:"req_bytes"`
	RespBytes   int64             `json:"resp_bytes"`
	LocalPort   int               `json:"local_port"`
}

// New constructs a Server.
func New(cfg Config) *Server {
	return &Server{
		cfg:      cfg,
		tunStats: map[string]*tunnelStat{},
		watchers: map[chan logEntry]struct{}{},
	}
}

// clientConn is one authenticated WebSocket connection from an intranet client.
type clientConn struct {
	server    *Server
	localPort int
	subdomain string
	ws        *websocket.Conn
	writeMu   sync.Mutex
	pending   sync.Map
}

func (cc *clientConn) send(m *protocol.Message) error {
	cc.writeMu.Lock()
	defer cc.writeMu.Unlock()
	return cc.ws.WriteJSON(m)
}

func randomToken(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func randomSubdomain() string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	for i := range b {
		b[i] = letters[int(b[i])%len(letters)]
	}
	return string(b)
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

const requestTimeout = 30 * time.Second

func (s *Server) recordRequest(e logEntry) {
	s.statsMu.Lock()
	st := s.tunStats[e.Subdomain]
	if st == nil {
		st = &tunnelStat{}
		s.tunStats[e.Subdomain] = st
	}
	st.Requests++
	st.ReqBytes += e.ReqBytes
	st.RespBytes += e.RespBytes
	st.LastSeen = e.At

	s.logs = append(s.logs, e)
	if len(s.logs) > 200 {
		s.logs = s.logs[len(s.logs)-200:]
	}
	watchers := make([]chan logEntry, 0, len(s.watchers))
	for ch := range s.watchers {
		watchers = append(watchers, ch)
	}
	s.statsMu.Unlock()

	for _, ch := range watchers {
		select {
		case ch <- e:
		default:
		}
	}
}

// OnlineTunnel is the public view of an active tunnel.
type OnlineTunnel struct {
	Subdomain string `json:"subdomain"`
	LocalPort int    `json:"local_port"`
	PublicURL string `json:"public_url"`
}

// StatsResponse is the payload for /api/stats.
type StatsResponse struct {
	OnlineTunnels []OnlineTunnel        `json:"online_tunnels"`
	TotalRequests int64                 `json:"total_requests"`
	ReqBytes      int64                 `json:"req_bytes"`
	RespBytes     int64                 `json:"resp_bytes"`
	RecentLogs    []logEntry            `json:"recent_logs"`
	PerTunnel     map[string]tunnelStat `json:"per_tunnel"`
	BaseDomain    string                `json:"base_domain"`
}
