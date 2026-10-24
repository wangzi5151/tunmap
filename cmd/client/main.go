package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/tunmap/tunmap/internal/client"
)

func main() {
	var (
		cfgPath   = flag.String("config", "", "config file path (e.g. ~/.tunmap.ini)")
		serverURL = flag.String("server", "", "public server URL, e.g. https://example.com")
		key       = flag.String("key", "", "shared key set on the server")
		port      = flag.Int("port", 0, "intranet local port to expose, e.g. 5050")
		subdomain = flag.String("subdomain", "", "optional custom subdomain")
	)
	flag.Parse()

	cfg := map[string]string{}
	if *cfgPath != "" {
		var err error
		if cfg, err = loadConfig(*cfgPath); err != nil {
			log.Fatalf("read config: %v", err)
		}
	} else if home, err := os.UserHomeDir(); err == nil {
		if _, err := os.Stat(home + "/.tunmap.ini"); err == nil {
			cfg, _ = loadConfig(home + "/.tunmap.ini")
		}
	}

	pick := func(flagVal, envKey, cfgKey string) string {
		if flagVal != "" {
			return flagVal
		}
		if v := os.Getenv(envKey); v != "" {
			return v
		}
		return cfg[cfgKey]
	}

	finalServer := pick(*serverURL, "TUNMAP_SERVER", "server")
	if finalServer == "" {
		finalServer = "http://localhost:8080"
	}
	finalKey := pick(*key, "TUNMAP_KEY", "key")
	finalSub := pick(*subdomain, "TUNMAP_SUBDOMAIN", "subdomain")
	finalPort := *port
	if finalPort == 0 {
		if v, err := fmtAtoi(os.Getenv("TUNMAP_PORT")); err == nil {
			finalPort = v
		} else if v, err := fmtAtoi(cfg["port"]); err == nil {
			finalPort = v
		}
	}

	if finalPort == 0 {
		log.Fatal("usage: tunmap-client --port 5050 [--server https://example.com] [--key <optional>] [--subdomain myapp]\n" +
			"       or set TUNMAP_SERVER / TUNMAP_PORT env vars,\n" +
			"       or write ~/.tunmap.ini with keys: server, key, port, subdomain")
	}

	log.Printf("tunmap client → %s (exposing localhost:%d, subdomain %q)", finalServer, finalPort, orDash(finalSub))
	client.Run(client.Options{
		Server:    finalServer,
		Key:       finalKey,
		LocalPort: finalPort,
		Subdomain: finalSub,
	}, nil)
}

func loadConfig(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	out := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		kv := strings.SplitN(line, ":", 2)
		if len(kv) == 1 {
			kv = strings.SplitN(line, "=", 2)
		}
		if len(kv) != 2 {
			continue
		}
		out[strings.TrimSpace(kv[0])] = strings.Trim(strings.TrimSpace(kv[1]), `"'`)
	}
	return out, sc.Err()
}

func fmtAtoi(s string) (int, error) {
	var n int
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}

func orDash(s string) string {
	if s == "" {
		return "(auto)"
	}
	return s
}
