package main

import (
	"flag"
	"log"
	"os"

	"github.com/tunmap/tunmap/internal/server"
)

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func main() {
	addr := flag.String("addr", envOr("TUNMAP_ADDR", ":8080"), "listen address")
	baseDomain := flag.String("domain", envOr("TUNMAP_BASE_DOMAIN", "localhost"), "base domain, e.g. example.com")
	authKey := flag.String("key", envOr("TUNMAP_AUTH_KEY", ""), "optional shared key; without it the server is open")
	flag.Parse()

	srv := server.New(server.Config{
		Addr:       *addr,
		BaseDomain: *baseDomain,
		AuthKey:    *authKey,
	})
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
