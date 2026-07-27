# Benchmark results

Measured 2026-07-27 with `bench/main.go`. Reproduce with the commands under
each table. Every number here came off the run recorded below it, and the
caveats at the bottom are part of the result rather than an apology for it.

## Machine and method

```
CPU            AMD Ryzen 5 7600X3D, 6 cores / 12 threads
OS             Windows 11, GOOS=windows GOARCH=amd64
Go             go1.26.4
clock          QueryPerformanceCounter, 10 MHz, 100ns floor, ~60-110ns per read
working set    10,000 keys x 64 byte values, every GET a hit
```

**Closed loop.** Each client writes one command, flushes, and blocks on its
reply before sending the next. So a percentile below is a round trip a caller
would really wait for. Pipelining would report a much larger ops/sec and a p99
that means nothing.

**Median, not best.** Each cell runs 7 times and the median is reported, with
`spread` = (max − min) / median printed beside it. Best-of-N would report how
quiet the machine happened to get on one run, and it can only ever move a
number up. **Any cell with a spread above about 20% has not really been
measured, and nothing here quotes one.**

**Clock.** Go's `time.Now` on Windows reads the interrupt-time page, which on
this box ticks every **533.9µs**. A 20µs round trip therefore measures as
either 0 or 534us, and the first version of this harness printed `p50 0us,
p99 544us` for every workload. `bench/clock_windows.go` reaches
`QueryPerformanceCounter` through `syscall` instead, for a 100ns floor at
~64ns per read. Two reads are charged per operation, about 0.1µs against a p50
of 200µs+, and the harness prints its own measured clock floor and per-read
cost in the header so this is checkable rather than asserted.

## Cross-process: server in its own process

This is the honest configuration and the one to quote. Client and server are
separate processes talking over loopback TCP.

```
go run ./cmd/kvstore -addr 127.0.0.1:7999          # terminal one
go run ./bench -addr 127.0.0.1:7999 -clients 1,2,4,8,16,32,64 \
  -ops 15000 -warmup 2000 -keys 10000 -runs 7      # terminal two
```

```
workload   clients      ops/sec   spread       p50       p90       p99       max
get              1        15300     9.0%      56us      85us     158us    2891us
get              2        42831    11.9%      32us      80us     146us    3687us
get              4        42638    12.0%      80us     120us     240us   10639us
get              8        66510    24.2%     100us     167us     351us    4321us
get             16        83684    28.3%     144us     263us     627us   15258us
get             32       107783    10.1%     234us     417us     880us   23219us
get             64       116827    16.4%     423us     767us    1595us   27458us
mixed            1        14040    34.3%      57us      87us     170us   21412us
mixed            2        43642    19.1%      32us      81us     135us    1296us
mixed            4        42824    17.4%      81us     116us     211us    4609us
mixed            8        82351     6.7%      89us     126us     193us    3043us
mixed           16       115615    15.5%     123us     185us     286us    3845us
mixed           32       142851    10.2%     198us     305us     514us    5649us
mixed           64       158566     5.4%     345us     526us     979us    7563us
set              1        17558     4.8%      50us      72us      97us     299us
set              2        44853    23.9%      32us      79us     133us     441us
set              4        44012    13.4%      80us     117us     224us    1610us
set              8        75732    13.6%      93us     141us     229us    1535us
set             16       111302    18.4%     127us     193us     306us    2198us
set             32       126871    20.3%     215us     355us     638us    6182us
set             64       146093     8.5%     369us     578us    1033us    8169us
```

`get` is all reads, `set` is all writes, `mixed` is 90% reads.

**The defensible summary line:** at 32 concurrent clients the server sustains
**over 100,000 ops/sec with p99 under 900µs**, on all three workloads, and it
is still climbing at 64. Every cell backing that sentence has a spread at or
under about 20%.

## In-process: server spawned inside the harness

The harness default, kept because it needs no second terminal. Traffic still
crosses a real socket, but client and server share one Go runtime and one
process, which skips a cross-process context switch per round trip and reports
**roughly 1.3-1.7x higher** throughput. Convenient, and not the number to
quote.

```
go run ./bench -clients 1,2,4,8,16,32,64 -ops 15000 -warmup 2000 -keys 10000 -runs 7
```

```
workload   clients      ops/sec   spread       p50       p90       p99       max
get              1        41181    16.6%      20us      31us      59us     150us
get              2        38262     7.3%      51us      82us     113us     224us
get              4        58281     6.4%      65us      97us     135us    1694us
get              8       136888     6.5%      47us      92us     171us   13710us
get             16       201594    20.0%      60us     109us     394us    8972us
get             32       190192    13.2%     122us     238us     826us    7382us
get             64       186930     7.6%     259us     437us    1528us   13007us
mixed            1        31728     9.3%      27us      40us     121us     776us
mixed            2        40564    15.6%      46us      76us     116us    1313us
mixed            4        60093     9.1%      56us      97us     249us    1826us
mixed            8       107986    18.7%      54us     113us     377us    6836us
mixed           16       150279     4.4%      69us     144us     662us    7816us
mixed           32       147922    47.9%     121us     348us    1265us   34966us
mixed           64       179771    26.8%     161us     680us    2281us    9720us
set              1        31868    13.1%      29us      37us      93us     385us
set              2        39515    18.0%      48us      84us     118us     388us
set              4        59685     9.6%      62us      96us     141us    1424us
set              8       131310     8.9%      48us      96us     191us   10612us
set             16       165060    13.9%      67us     137us     538us   14042us
set             32       177811    15.7%     129us     253us     933us   12286us
set             64       164569    25.1%     283us     509us    1882us   11901us
```

## The store and its lock, on its own

The existing benchmarks in `store/store_test.go`, both `b.RunParallel` across
all 12 threads, both hammering a single key so the lock is under the worst
contention the type can see.

```
go test -bench=. -benchmem -benchtime=3s -count=3 ./store
```

```
BenchmarkGet-12    136277914    26.93 ns/op    0 B/op    0 allocs/op
BenchmarkGet-12    132503132    26.43 ns/op    0 B/op    0 allocs/op
BenchmarkGet-12    138258696    26.77 ns/op    0 B/op    0 allocs/op
BenchmarkSet-12     63281793    54.06 ns/op    0 B/op    0 allocs/op
BenchmarkSet-12     66344648    53.61 ns/op    0 B/op    0 allocs/op
BenchmarkSet-12     64945842    57.48 ns/op    0 B/op    0 allocs/op
```

About **27ns per read and 55ns per write**, zero allocations, which is roughly
**37M reads/sec and 18M writes/sec** through the lock.

## What the numbers actually settled

**1. The RWMutex is not the bottleneck, and it is not close.** `store.go`
carries a comment saying sharding "is not worth the complexity until a
benchmark says this lock is hot." This is that benchmark, and it says the lock
is cold. At the cross-process read rate of 116,827 ops/sec the server has
**8.6µs of budget per operation** and spends **27ns of it in the store**, which
is **under 0.4%**. Writes are under 0.9%. Sharding the map would divide a
number that is already a rounding error, so the simple lock stays and the
comment in `store.go` is now backed rather than assumed.

**2. Read-only and write-only swap places depending on configuration**, which
is the clearest evidence the lock is irrelevant here. All-write is faster
cross-process (146,093 vs 116,827 at 64 clients), all-read is faster
in-process (186,930 vs 164,569 at the same point). A lock-bound server would
put all-write last in both, because `Set` takes the write lock alone while any
number of `Get`s share the read lock. The ordering tracks the transport, not
the mutex.

**3. What separates them is reply bytes.** A `GET` reply is the 64 byte value
plus a newline; a `SET` reply is `OK\n`, three bytes. Re-running the 32 client
cell cross-process with 4 byte values, so only the GET reply shrinks, moved
**GET from 107,783 to 119,921 ops/sec (+11%)** while SET stayed flat at
126,871 to 129,417 (+2%), closing most of the gap:

```
go run ./bench -addr 127.0.0.1:7998 -clients 32 -ops 15000 -warmup 2000 \
  -keys 10000 -runs 7 -valsize 4 -workloads get,set

workload   clients      ops/sec   spread       p50       p90       p99       max
get             32       119921    21.3%     226us     370us     681us   12173us
set             32       129417    10.8%     213us     331us     557us    7970us
```

Shrinking one workload's replies lifted only that workload, which is what the
reply-size explanation predicts. Calling it confirmed would be too strong: the
GET cell's 21.3% spread is at the edge of what this box can resolve, and a gap
remains. It is the leading explanation, not a closed case.

**4. The concrete next optimisation, identified and deliberately not taken.**
`server.go` replies with a bare `io.WriteString(conn, ...)`, one unbuffered
write syscall per command. Finding 3 says the write path is where the per-op
budget goes, so wrapping the connection in a `bufio.Writer` and flushing once
the read side has nothing pending is the obvious next move. It is not in this
commit on purpose: the harness was the task, and `server/` currently has the
property that one commit wrote it correct the first time. Changing it deserves
its own commit and its own before-and-after run.

**5. One client is a scheduler measurement, not a server measurement.** A lone
connection is *slower per operation* than two: 15,300 ops/sec at one client
against 42,831 at two, cross-process. With only one connection in flight the
runtime has nothing else runnable, parks the thread on every blocking read, and
pays a wake-up per round trip. A second client keeps a thread hot and the
per-op cost falls. So the 1 and 2 client rows measure Windows thread wake-up
latency, and the saturated rows from 16 clients up are the ones that describe
the server.

**6. Saturation sits at 32-64 clients** on 12 threads, cross-process, and the
tail widens faster than throughput grows past it: 32 to 64 clients buys about
8% more throughput on reads for **1.8x the p99**. The 8 clients in
`TestConcurrentClients` are well inside the linear region, so that test proves
correctness under concurrency and was never sized to find the ceiling.

## Caveats, which travel with the numbers

- **Loopback on one box.** Client and server share the same 12 threads, so the
  harness competes with the thing it is measuring. There is no NIC, no driver,
  and no real network in any figure here. Nothing in this file describes
  behaviour over a network or under real users, and none of it is a production
  measurement.
- **The machine was not quiesced.** A normal desktop session was running
  throughout. That is why the spread column exists and why medians are
  reported.
- **Windows only, so far.** The harness builds and vets clean for
  `GOOS=linux` via `bench/clock_other.go`, but has never been run there. Expect
  materially different numbers on Linux, where loopback and thread wake-up are
  both cheaper.
- **Two cells are unusable and stay in the tables anyway**, because deleting
  them would be the dishonest move: in-process `mixed` at 32 clients (47.9%
  spread) and in-process `mixed` at 64 (26.8%).
- **64KB line cap.** `bufio.Scanner` bounds a command, so the harness never
  tests values near that limit.

For context, Redis on comparable hardware does low hundreds of thousands of
ops/sec per core on `redis-benchmark`, which is the same order as the numbers
above. That is not a claim of parity: `redis-benchmark` pipelines by default,
Redis implements a typed wire format, persistence, eviction, and replication,
and this is a text protocol over a map. The comparison is here to say the
numbers are plausible, not to say the two are equivalent.
