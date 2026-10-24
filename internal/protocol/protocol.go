// Package protocol defines the JSON messages exchanged between the
// public server and the intranet client over a single WebSocket.
package protocol

// Message types, sent in the "type" field of every JSON frame.
const (
	TypeTunnelCreate  = "tunnel_create"   // client -> server
	TypeTunnelCreated = "tunnel_created"  // server -> client
	TypeTunnelError   = "tunnel_error"    // server -> client
	TypeHTTPRequest   = "http_request"   // server -> client
	TypeHTTPResponse  = "http_response"   // client -> server
	TypePing          = "ping"            // bidirectional keepalive
	TypePong          = "pong"
)

// Message is a single frame on the control/data WebSocket.
// Exactly one of the typed payload structs is filled per message.
type Message struct {
	Type string `json:"type"`

	// tunnel_create (client -> server)
	LocalPort int    `json:"local_port,omitempty"`
	Subdomain string `json:"subdomain,omitempty"` // optional custom subdomain

	// tunnel_created / tunnel_error
	PublicURL string `json:"public_url,omitempty"`
	Message   string `json:"message,omitempty"`

	// http_request (server -> client) / http_response (client -> server)
	ID      string            `json:"id,omitempty"`
	Method  string            `json:"method,omitempty"`
	Path    string            `json:"path,omitempty"`
	Status  int               `json:"status,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    string            `json:"body,omitempty"` // base64 encoded
}
