// Command bench is a closed-loop load harness for the kvstore server.
//
// It answers two questions the code could only guess at: how many ops a
// second the server sustains, and whether the store's single RWMutex is
// the thing in the way. Every request here crosses a real TCP socket and
// the real line protocol, so the numbers include framing and syscalls
// rather than measuring the map in isolation. The store benchmarks in
// store/store_test.go cover the lock on its own, and the gap between the
// two is the answer to the second question.
//
// Closed loop means each client sends one command and waits for its
// reply before sending the next, so a percentile here is a round trip a
// caller would actually feel. Pipelining would report a bigger ops/sec
// and a meaningless p99.
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"math/rand"
	"net"
	"os"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"kvstore/server"
	"kvstore/store"
)

func main() {
	var (
		addr      = flag.String("addr", "", "server to hit, empty spawns one in this process")
		clients   = flag.String("clients", "1,8,64", "client counts to sweep")
		ops       = flag.Int("ops", 20000, "measured ops per client")
		warmup    = flag.Int("warmup", 2000, "unmeasured ops per client before the timer starts")
		keys      = flag.Int("keys", 10000, "distinct keys in the working set")
		valSize   = flag.Int("valsize", 64, "value length in bytes")
		runs      = flag.Int("runs", 3, "repeats per cell, best ops/sec reported with the spread")
		workloads = flag.String("workloads", "get,mixed,set", "any of get, set, mixed")
	)
	flag.Parse()

	target := *addr
	if target == "" {
		srv := server.New(store.NewMemoryStore())
		if err := srv.Listen("127.0.0.1:0"); err != nil {
			fail("listen: %v", err)
		}
		go srv.Serve()
		defer srv.Shutdown(context.Background())
		target = srv.Addr()
	}

	counts, err := parseInts(*clients)
	if err != nil {
		fail("clients: %v", err)
	}
	mixes, err := parseWorkloads(*workloads)
	if err != nil {
		fail("%v", err)
	}

	value := strings.Repeat("x", *valSize)
	if err := populate(target, *keys, value); err != nil {
		fail("populate: %v", err)
	}

	res, cost := probeClock()

	fmt.Printf("kvstore load harness\n")
	fmt.Printf("  go            %s %s/%s\n", runtime.Version(), runtime.GOOS, runtime.GOARCH)
	fmt.Printf("  cpus          %d\n", runtime.NumCPU())
	fmt.Printf("  clock         %s, %v floor, %v per read, 2 reads charged per op\n", clockName(), res, cost)
	fmt.Printf("  target        %s\n", target)
	fmt.Printf("  working set   %d keys x %d byte values\n", *keys, *valSize)
	fmt.Printf("  per run       %d measured ops/client after %d warmup\n", *ops, *warmup)
	fmt.Printf("  runs          %d per cell, median reported, closed loop, one outstanding request per client\n\n", *runs)

	fmt.Printf("%-9s %8s %12s %8s %9s %9s %9s %9s\n",
		"workload", "clients", "ops/sec", "spread", "p50", "p90", "p99", "max")

	for _, w := range mixes {
		for _, n := range counts {
			best, spread, err := repeat(*runs, func() (result, error) {
				return measure(target, n, *ops, *warmup, *keys, value, w.readRatio)
			})
			if err != nil {
				fail("%s at %d clients: %v", w.name, n, err)
			}
			fmt.Printf("%-9s %8d %12.0f %7.1f%% %9s %9s %9s %9s\n",
				w.name, n, best.opsPerSec(), spread*100,
				dur(best.pct(50)), dur(best.pct(90)), dur(best.pct(99)), dur(best.max()))
		}
	}
}

type workload struct {
	name      string
	readRatio float64 // share of ops that are GETs
}

func parseWorkloads(s string) ([]workload, error) {
	var out []workload
	for _, name := range strings.Split(s, ",") {
		name = strings.TrimSpace(name)
		switch name {
		case "":
			continue
		case "get":
			out = append(out, workload{name, 1})
		case "set":
			out = append(out, workload{name, 0})
		case "mixed":
			// 90/10 is the read heavy shape a cache in front of a
			// database sees, and it is where an RWMutex is supposed to
			// pay off over a plain Mutex.
			out = append(out, workload{name, 0.9})
		default:
			return nil, fmt.Errorf("unknown workload %q", name)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no workloads given")
	}
	return out, nil
}

type result struct {
	ops int
	dur time.Duration
	lat []time.Duration // sorted ascending
}

func (r result) opsPerSec() float64 { return float64(r.ops) / r.dur.Seconds() }

func (r result) max() time.Duration {
	if len(r.lat) == 0 {
		return 0
	}
	return r.lat[len(r.lat)-1]
}

// pct is nearest rank on the sorted sample with no interpolation, so
// every latency printed is one that was really observed.
func (r result) pct(p float64) time.Duration {
	if len(r.lat) == 0 {
		return 0
	}
	i := int(p / 100 * float64(len(r.lat)))
	if i >= len(r.lat) {
		i = len(r.lat) - 1
	}
	return r.lat[i]
}

// repeat runs one cell several times and returns the median result plus
// the spread of the throughputs, as (max-min)/median.
//
// Median and not best of N. Best of N reports how quiet the machine got
// once, which is a fact about the machine rather than about the server,
// and it only ever moves the number up. The spread is printed next to
// every cell for the same reason: a cell with a wide spread has not been
// measured, and quoting it anywhere would be quoting noise.
func repeat(n int, f func() (result, error)) (result, float64, error) {
	rs := make([]result, 0, n)
	for i := 0; i < n; i++ {
		r, err := f()
		if err != nil {
			return result{}, 0, err
		}
		rs = append(rs, r)
	}
	sort.Slice(rs, func(i, j int) bool { return rs[i].opsPerSec() < rs[j].opsPerSec() })
	med := rs[len(rs)/2]
	if med.opsPerSec() == 0 {
		return med, 0, nil
	}
	spread := (rs[len(rs)-1].opsPerSec() - rs[0].opsPerSec()) / med.opsPerSec()
	return med, spread, nil
}

// measure drives n concurrent clients through warmup then ops each and
// times the measured phase only. Latencies stay in per client slices so
// recording them never becomes its own contention point.
func measure(addr string, n, ops, warmup, keys int, value string, readRatio float64) (result, error) {
	conns := make([]*client, n)
	for i := range conns {
		c, err := dial(addr, i, keys, value, readRatio)
		if err != nil {
			closeAll(conns)
			return result{}, err
		}
		conns[i] = c
	}
	defer closeAll(conns)

	var (
		wg    sync.WaitGroup
		start = make(chan struct{})
		errs  = make([]error, n)
	)
	for i, c := range conns {
		wg.Add(1)
		go func(i int, c *client) {
			defer wg.Done()
			if err := c.run(warmup, false); err != nil {
				errs[i] = err
				return
			}
			<-start
			errs[i] = c.run(ops, true)
		}(i, c)
	}

	// Warmups finish at their own pace, so the timer covers the measured
	// phase only and starts once every client is parked on start.
	time.Sleep(100 * time.Millisecond)
	t0 := nanos()
	close(start)
	wg.Wait()
	elapsed := time.Duration(nanos() - t0)

	all := make([]time.Duration, 0, n*ops)
	for i, c := range conns {
		if errs[i] != nil {
			return result{}, errs[i]
		}
		all = append(all, c.lat...)
	}
	sort.Slice(all, func(i, j int) bool { return all[i] < all[j] })
	return result{ops: len(all), dur: elapsed, lat: all}, nil
}

type client struct {
	conn      net.Conn
	r         *bufio.Reader
	w         *bufio.Writer
	rng       *rand.Rand
	keys      int
	value     string
	readRatio float64
	lat       []time.Duration
}

func dial(addr string, seed, keys int, value string, readRatio float64) (*client, error) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}
	if tc, ok := conn.(*net.TCPConn); ok {
		// Nagle would batch these small commands and turn a latency
		// measurement into a measurement of the delayed ack timer.
		tc.SetNoDelay(true)
	}
	return &client{
		conn:      conn,
		r:         bufio.NewReader(conn),
		w:         bufio.NewWriter(conn),
		rng:       rand.New(rand.NewSource(int64(seed) + 1)),
		keys:      keys,
		value:     value,
		readRatio: readRatio,
	}, nil
}

func (c *client) run(n int, record bool) error {
	if record {
		c.lat = make([]time.Duration, 0, n)
	}
	for i := 0; i < n; i++ {
		key := "key" + itoa(c.rng.Intn(c.keys))
		line := "GET " + key + "\n"
		if c.rng.Float64() >= c.readRatio {
			line = "SET " + key + " " + c.value + "\n"
		}
		t0 := nanos()
		if _, err := c.w.WriteString(line); err != nil {
			return err
		}
		if err := c.w.Flush(); err != nil {
			return err
		}
		reply, err := c.r.ReadString('\n')
		if err != nil {
			return err
		}
		if record {
			c.lat = append(c.lat, time.Duration(nanos()-t0))
		}
		// A GET here is always a hit and a SET always replies OK, so an
		// error line means the harness and the server disagree about the
		// protocol and every number after it would be junk.
		if strings.HasPrefix(reply, "ERR ") || reply == "(nil)\n" {
			return fmt.Errorf("unexpected reply %q to %q", strings.TrimSpace(reply), strings.TrimSpace(line))
		}
	}
	return nil
}

func closeAll(cs []*client) {
	for _, c := range cs {
		if c != nil {
			c.conn.Close()
		}
	}
}

// populate seeds the keyspace so a GET workload is all hits. A miss
// returns early with "(nil)" and never reads the map, which would
// flatter the read numbers.
func populate(addr string, keys int, value string) error {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return err
	}
	defer conn.Close()
	w := bufio.NewWriter(conn)
	r := bufio.NewReader(conn)
	for i := 0; i < keys; i++ {
		if _, err := w.WriteString("SET key" + itoa(i) + " " + value + "\n"); err != nil {
			return err
		}
		if err := w.Flush(); err != nil {
			return err
		}
		if _, err := r.ReadString('\n'); err != nil {
			return err
		}
	}
	return nil
}

func parseInts(s string) ([]int, error) {
	var out []int
	for _, f := range strings.Split(s, ",") {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		var n int
		if _, err := fmt.Sscanf(f, "%d", &n); err != nil || n < 1 {
			return nil, fmt.Errorf("bad count %q", f)
		}
		out = append(out, n)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no counts given")
	}
	return out, nil
}

// itoa keeps the hot loop off fmt.Sprintf, which showed up as harness
// overhead rather than server time when the client count was low.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

// probeClock measures the timing floor, so a reader can check the
// percentiles against it rather than trust them. res is the smallest
// nonzero gap two back to back reads resolve, cost is what one read
// charges the thing it is measuring. Percentiles near res are the clock
// talking, not the server.
func probeClock() (res, cost time.Duration) {
	const n = 100000
	lo := int64(1) << 62
	for i := 0; i < n; i++ {
		a := nanos()
		b := nanos()
		if d := b - a; d > 0 && d < lo {
			lo = d
		}
	}
	t0 := nanos()
	for i := 0; i < n; i++ {
		_ = nanos()
	}
	return time.Duration(lo), time.Duration((nanos() - t0) / n)
}

// dur prints microseconds because every latency here is well under a
// millisecond and Duration's own formatting switches units mid column.
func dur(d time.Duration) string {
	return fmt.Sprintf("%.0fus", float64(d.Microseconds()))
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "bench: "+format+"\n", args...)
	os.Exit(1)
}
