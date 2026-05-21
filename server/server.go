// Package server owns the TCP loop, one goroutine per connection, and
// graceful shutdown.
package server

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"sync"

	"kvstore/protocol"
	"kvstore/store"
)

type Server struct {
	st store.Store
	ln net.Listener

	mu    sync.Mutex
	conns map[net.Conn]struct{}
	wg    sync.WaitGroup
	done  chan struct{}
}

func New(st store.Store) *Server {
	return &Server{
		st:    st,
		conns: make(map[net.Conn]struct{}),
		done:  make(chan struct{}),
	}
}

// Listen binds the address and returns once the socket is ready, separate
// from Serve so tests can bind port 0 and read the real address back.
func (s *Server) Listen(addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	s.ln = ln
	return nil
}

func (s *Server) Addr() string {
	return s.ln.Addr().String()
}

// Serve accepts until Shutdown closes the listener. Goroutine per
// connection is cheap in Go, the runtime parks blocked reads on its
// poller, so idle connections cost small stacks and not OS threads.
func (s *Server) Serve() error {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			// Shutdown unblocks Accept by closing the listener, so once
			// done is closed this error is expected.
			select {
			case <-s.done:
				return nil
			default:
				return err
			}
		}
		// Admission and the shutdown check share one critical section,
		// otherwise a connection accepted mid shutdown is never closed
		// and its wg.Add races the wg.Wait in the drain goroutine.
		s.mu.Lock()
		select {
		case <-s.done:
			s.mu.Unlock()
			conn.Close()
			return nil
		default:
		}
		s.conns[conn] = struct{}{}
		s.wg.Add(1)
		s.mu.Unlock()
		go s.handle(conn)
	}
}

func (s *Server) handle(conn net.Conn) {
	defer s.wg.Done()
	defer func() {
		s.mu.Lock()
		delete(s.conns, conn)
		s.mu.Unlock()
		conn.Close()
	}()

	// Scanner caps lines at 64KB. Fine for a toy protocol, a real server
	// would need explicit framing and a length limit it controls.
	sc := bufio.NewScanner(conn)
	for sc.Scan() {
		cmd, err := protocol.Parse(sc.Text())
		if err != nil {
			if _, werr := io.WriteString(conn, protocol.ReplyError(err)); werr != nil {
				return
			}
			continue
		}
		if _, err := io.WriteString(conn, s.execute(cmd)); err != nil {
			return
		}
	}
	// A vanished client or Shutdown closing the socket both just end the
	// scan, neither needs handling.
}

func (s *Server) execute(cmd protocol.Command) string {
	switch cmd.Op {
	case protocol.OpGet:
		v, ok := s.st.Get(cmd.Key)
		if !ok {
			return protocol.ReplyNil()
		}
		return protocol.ReplyValue(v)
	case protocol.OpSet:
		s.st.Set(cmd.Key, cmd.Value)
		return protocol.ReplyOK()
	case protocol.OpDel:
		if s.st.Delete(cmd.Key) {
			return protocol.ReplyInt(1)
		}
		return protocol.ReplyInt(0)
	}
	// Parse only emits the three ops above, reaching here is a bug.
	return protocol.ReplyError(errUnhandled)
}

var errUnhandled = errors.New("unhandled op")

// Shutdown stops accepting, closes live connections so their reads
// unblock, then waits for every handler to drain or the context to give
// up. Call it once.
func (s *Server) Shutdown(ctx context.Context) error {
	// Same lock Serve admits under, so after this block every connection
	// was either closed here or refused at the gate.
	s.mu.Lock()
	close(s.done)
	s.ln.Close()
	for c := range s.conns {
		c.Close()
	}
	s.mu.Unlock()

	drained := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(drained)
	}()
	select {
	case <-drained:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
