// Integration tests that bind a real port and talk to the server over TCP.
//
// Port 0 lets the OS pick a free port so tests never collide.
package server

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"kvstore/store"
)

// startServer runs a real server on a free port, torn down with the test.
func startServer(t *testing.T) string {
	t.Helper()
	srv := New(store.NewMemoryStore())
	if err := srv.Listen("127.0.0.1:0"); err != nil {
		t.Fatalf("listen: %v", err)
	}
	go srv.Serve()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			t.Errorf("shutdown: %v", err)
		}
	})
	return srv.Addr()
}

func dial(t *testing.T, addr string) (net.Conn, *bufio.Reader) {
	t.Helper()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn, bufio.NewReader(conn)
}

func roundTrip(t *testing.T, conn net.Conn, r *bufio.Reader, cmd string) string {
	t.Helper()
	if _, err := fmt.Fprintf(conn, "%s\n", cmd); err != nil {
		t.Fatalf("write %q: %v", cmd, err)
	}
	line, err := r.ReadString('\n')
	if err != nil {
		t.Fatalf("read reply to %q: %v", cmd, err)
	}
	return line
}

func TestCommandsOverTCP(t *testing.T) {
	addr := startServer(t)
	conn, r := dial(t, addr)

	steps := []struct{ cmd, want string }{
		{"SET name alice", "OK\n"},
		{"GET name", "alice\n"},
		{"SET name alice smith", "OK\n"},
		{"GET name", "alice smith\n"},
		{"DEL name", "1\n"},
		{"GET name", "(nil)\n"},
		{"DEL name", "0\n"},
		{"BOGUS x", "ERR unknown command \"BOGUS\"\n"},
		{"GET", "ERR GET takes exactly one key\n"},
	}
	for _, s := range steps {
		if got := roundTrip(t, conn, r, s.cmd); got != s.want {
			t.Fatalf("%q replied %q, want %q", s.cmd, got, s.want)
		}
	}
}

// Two connections must see the same data, the store is shared and the
// protocol is stateless.
func TestDataSharedAcrossConnections(t *testing.T) {
	addr := startServer(t)
	c1, r1 := dial(t, addr)
	c2, r2 := dial(t, addr)

	if got := roundTrip(t, c1, r1, "SET shared yes"); got != "OK\n" {
		t.Fatalf("SET replied %q", got)
	}
	if got := roundTrip(t, c2, r2, "GET shared"); got != "yes\n" {
		t.Fatalf("GET on second conn replied %q, want yes", got)
	}
}

// Many clients at once, full stack, run under -race. Each client writes
// to its own key so every reply is exactly predictable.
func TestConcurrentClients(t *testing.T) {
	addr := startServer(t)
	const clients = 8
	const ops = 50

	var wg sync.WaitGroup
	errc := make(chan error, clients)
	for c := 0; c < clients; c++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			conn, err := net.Dial("tcp", addr)
			if err != nil {
				errc <- fmt.Errorf("client %d dial: %v", id, err)
				return
			}
			defer conn.Close()
			r := bufio.NewReader(conn)
			key := fmt.Sprintf("k%d", id)
			for i := 0; i < ops; i++ {
				val := fmt.Sprintf("v%d", i)
				fmt.Fprintf(conn, "SET %s %s\n", key, val)
				if reply, _ := r.ReadString('\n'); reply != "OK\n" {
					errc <- fmt.Errorf("client %d SET reply %q", id, reply)
					return
				}
				fmt.Fprintf(conn, "GET %s\n", key)
				if reply, _ := r.ReadString('\n'); reply != val+"\n" {
					errc <- fmt.Errorf("client %d GET reply %q, want %q", id, reply, val)
					return
				}
			}
		}(c)
	}
	wg.Wait()
	close(errc)
	for err := range errc {
		t.Error(err)
	}
}

func TestShutdownClosesClients(t *testing.T) {
	srv := New(store.NewMemoryStore())
	if err := srv.Listen("127.0.0.1:0"); err != nil {
		t.Fatalf("listen: %v", err)
	}
	go srv.Serve()

	conn, err := net.Dial("tcp", srv.Addr())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown: %v", err)
	}

	// The closed socket surfaces to the client as EOF or a reset, either
	// way the read must not hang.
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 1)
	if _, err := conn.Read(buf); err == nil {
		t.Fatal("read after shutdown succeeded, want connection closed")
	}
}
