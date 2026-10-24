package server

import _ "embed"

// embeddedWeb holds the web console single-page app, compiled into the binary.
//
//go:embed web.html
var embeddedWeb []byte
