// Tests the HTTP bridge against a real kvstore server on a real socket.
package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"kvstore/server"
	"kvstore/store"
)

func startKV(t *testing.T) string {
	t.Helper()
	srv := server.New(store.NewMemoryStore())
	if err := srv.Listen("127.0.0.1:0"); err != nil {
		t.Fatalf("listen: %v", err)
	}
	go srv.Serve()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(ctx)
	})
	return srv.Addr()
}

func post(t *testing.T, h http.HandlerFunc, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", "/cmd", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h(rec, req)
	return rec
}

func TestBridgeRoundTrip(t *testing.T) {
	h := handleCmd(startKV(t))

	steps := []struct{ cmd, want string }{
		{"SET name alice smith", "OK"},
		{"GET name", "alice smith"},
		{"DEL name", "1"},
		{"GET name", "(nil)"},
	}
	for _, s := range steps {
		rec := post(t, h, `{"command":"`+s.cmd+`"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("%q returned status %d: %s", s.cmd, rec.Code, rec.Body)
		}
		var got result
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("%q bad json: %v", s.cmd, err)
		}
		if got.Reply != s.want {
			t.Fatalf("%q replied %q, want %q", s.cmd, got.Reply, s.want)
		}
	}
}

func TestBridgeRejectsBadInput(t *testing.T) {
	h := handleCmd(startKV(t))

	cases := []struct {
		name string
		body string
	}{
		{"not json", "hello"},
		{"empty command", `{"command":""}`},
		{"embedded newline", `{"command":"GET a\nGET b"}`},
	}
	for _, c := range cases {
		if rec := post(t, h, c.body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400", c.name, rec.Code)
		}
	}
}

func TestBridgeServerDown(t *testing.T) {
	// Nothing listens on port 1, so the dial fails fast.
	h := handleCmd("127.0.0.1:1")
	if rec := post(t, h, `{"command":"GET x"}`); rec.Code != http.StatusBadGateway {
		t.Fatalf("status %d, want 502", rec.Code)
	}
}
