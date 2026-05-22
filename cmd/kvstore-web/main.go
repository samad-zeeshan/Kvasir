// Command kvstore-web is a browser console for demoing the kvstore server.
//
// An ordinary TCP client behind an HTTP handler, the core server does not
// know it exists.
package main

import (
	"bufio"
	_ "embed"
	"encoding/json"
	"flag"
	"log"
	"net"
	"net/http"
	"strings"
	"time"
)

//go:embed index.html
var page []byte

func main() {
	kvAddr := flag.String("kv", "127.0.0.1:6380", "kvstore server address")
	httpAddr := flag.String("http", "127.0.0.1:8080", "web listen address")
	flag.Parse()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(page)
	})
	// POST rather than WebSocket, the protocol is strict request reply
	// and the standard library has no WebSocket, this stays zero deps.
	mux.HandleFunc("POST /cmd", handleCmd(*kvAddr))

	log.Printf("demo console on http://%s, forwarding to kvstore at %s", *httpAddr, *kvAddr)
	log.Fatal(http.ListenAndServe(*httpAddr, mux))
}

type result struct {
	Sent  string `json:"sent"`
	Reply string `json:"reply"`
}

// handleCmd forwards one command line over a fresh TCP connection and
// returns the one line reply. A dial per request is wasteful but leaves
// no shared connection to lock or reconnect, the right trade for a demo.
func handleCmd(kvAddr string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Command string `json:"command"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request body", http.StatusBadRequest)
			return
		}
		line := strings.TrimSpace(req.Command)
		// One line in, one line out. An embedded newline would smuggle a
		// second command past the pairing below, so reject it here.
		if line == "" || strings.ContainsAny(line, "\r\n") {
			http.Error(w, "send exactly one command line", http.StatusBadRequest)
			return
		}

		conn, err := net.DialTimeout("tcp", kvAddr, 2*time.Second)
		if err != nil {
			http.Error(w, "kvstore server is not reachable at "+kvAddr, http.StatusBadGateway)
			return
		}
		defer conn.Close()
		conn.SetDeadline(time.Now().Add(2 * time.Second))

		if _, err := conn.Write([]byte(line + "\n")); err != nil {
			http.Error(w, "write to kvstore failed", http.StatusBadGateway)
			return
		}
		reply, err := bufio.NewReader(conn).ReadString('\n')
		if err != nil {
			http.Error(w, "read from kvstore failed", http.StatusBadGateway)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result{Sent: line, Reply: strings.TrimRight(reply, "\r\n")})
	}
}
