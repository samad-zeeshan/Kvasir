# Notes for the demo page

Working notes behind `docs/index.html`. Everything here is drawn from files in
this repository. Where a number appears, the file it came from is named.

## What this actually does, in one sentence

You give it a label and a piece of text, and later you say the label and it
hands the text straight back, instantly, for as long as it is running.

## Who has the problem, and the moment they are stuck

A developer looking at a slow page on a Tuesday afternoon. The log shows the
same lookup on every visit, fetch this customer's display name, and the name
has not changed in weeks. The database is asked again on every visit anyway,
and every visitor waits while the answer comes off a disk. The developer wants
somewhere to keep that answer that is faster than a disk and visible to every
copy of the app, not just to the one process that happened to work it out.

## What people did before this existed

Two things, both worse.

- **Ask the database every time.** Correct, and slow. The answer is on a disk
  and the disk is the slowest part of the machine.
- **Keep the answer in a variable inside the app.** Fast, but private. Run the
  app on three machines and you get three different answers, and no way to
  clear them all at once.

A separate small server holding the answers fixes both: it is in memory, so it
is fast, and it is one shared place, so everybody sees the same value. That is
the idea Redis and memcached made ordinary. kvstore is that idea built from
scratch in Go with nothing but the standard library, to see how the concurrency
part actually works.

## The numbers in this repository, and how they were measured

All of these come from `bench/RESULTS.md`, measured 2026-07-27.

**The machine.** AMD Ryzen 5 7600X3D, 6 cores and 12 threads, Windows 11,
Go 1.26.4. One desktop, in normal use at the time, not a quiet lab machine.

**The method.** Client and server are separate programs on that one machine,
talking over its loopback, which is the machine talking to itself. No real
network is involved anywhere. Each asking program waits for its answer before
asking again, so every timing below is a round trip somebody would actually
wait for. Each cell was run 7 times and the middle result is reported, with the
gap between fastest and slowest printed next to it. `bench/RESULTS.md` states
that any cell whose gap is above about 20% has not really been measured, and it
does not quote one. Neither does the page.

| Number on the page | Value | Where it comes from |
| --- | --- | --- |
| Answers per second | 107,783 | `bench/RESULTS.md`, cross-process table, row `get 32`, spread 10.1% |
| Slowest 1 answer in 100 | 880 millionths of a second | same row, p99 column |
| Share of time in the storage code | under 0.4% | `bench/RESULTS.md`, finding 1: 8.6 millionths of a second of budget per answer, 27 billionths spent in the map |
| One lookup in the map | about 27 billionths of a second | `bench/RESULTS.md`, store section, `BenchmarkGet` at 26.93 ns/op |
| Parser test cases | 13 | `protocol/protocol_test.go`, the `TestParse` table |
| Test results | 15 test functions, 28 results, 0 failures | `go test -v -count=1 ./...` run 2026-07-27 |

Two figures in `bench/RESULTS.md` are deliberately **not** on the page. The
in-process table reports 1.3 to 1.7 times higher throughput, and that file
itself says it is the convenient number rather than the honest one. And the two
cells in that table with a spread of 26.8% and 47.9% are called unusable by the
file that prints them.

## What it does not do

- **It never writes anything down.** Stop the program and every label is gone.
  `store/store.go` is a map and a lock. There is no save file and no load path
  anywhere in the repository.
- **It holds only what fits in memory.** Nothing is thrown out to make room, so
  a large enough pile of labels runs the machine out of memory.
- **It runs on one machine.** `server/server.go` opens one listener. There is no
  second copy keeping up.
- **A value spelled `(nil)` is indistinguishable from a missing label.** That
  ambiguity is the price of a protocol a person can read, and it is written down
  in `protocol/protocol.go` and in the README.
- **No security of any kind.** No passwords, no encryption, no limit on who may
  connect. It is not something to expose to the open internet.
- **A command line is capped at 64KB** by the scanner in `server/server.go`.

## The format: it runs in the page

The page does not replay a recording. `docs/wasm/main.go` is a thin bridge that
imports this repository's real `store` and `protocol` packages and hands one
function to the browser. It is compiled with `GOOS=js GOARCH=wasm` into
`docs/kvstore.wasm`, and the reader's typing goes through `protocol.Parse` and
`store.MemoryStore` exactly as it would on a server.

The bridge starts with the build constraint `//go:build js && wasm`, so
`go build ./...` and `go test ./...` at the repository root never see it. Both
were rerun after it was added and both still pass.

**What is not in the page, and the page says so:** the network layer. A browser
tab has no sockets, so `server/server.go`, its listener and its one small worker
per connection cannot exist there. What runs in a reader's tab is the storage
engine and the command parser.

### Size of the compiled file

| Measure | Bytes | Rounded |
| --- | --- | --- |
| Raw `docs/kvstore.wasm` | 2,692,397 | 2.6 MB |
| Gzipped, `gzip -9` | 770,905 | 753 KB |

Raw is over the 2 MB line and gzipped is comfortably under 1 MB, so it ships.
GitHub Pages compresses on the way out, so the gzipped figure is what a reader
downloads. Both figures are recorded in `docs/data/kvstore.json` as well.

Build command:

```
GOOS=js GOARCH=wasm go build -trimpath -ldflags="-s -w" -o docs/kvstore.wasm ./docs/wasm
```

`docs/wasm_exec.js` is copied unchanged from `C:\Program Files\Go\lib\wasm\wasm_exec.js`
(Go 1.26.4). It is the loader Go ships and is required to start the compiled file.

## Signature element

**The drawer.** The live contents of the running map drawn as a card drawer:
eight ruled slots, a hairline index rail, one label per slot read back out of
the Go map through the real `Get`, and a heavy brass spine on the slot the last
command touched. It fits because a key value store is one shared drawer of
labelled cards, `SET` files a card, `GET` reads one and `DEL` pulls one out, and
because the drawer here is not a picture of the map, it is the map: every line
is re-read from the running Go code after every command.

## Honesty checks made while building the page

- Every number on the page carries an HTML comment naming the repository file
  it came from.
- The captured session in `docs/data/kvstore.json` was taken from the real TCP
  server started with `go run ./cmd/kvstore -addr 127.0.0.1:7911` and driven by
  a scripted client. Those are the exact bytes it wrote back.
- `bench/RESULTS.md` contains two characters that did not survive an encoding
  step, a minus sign and a micro sign. Neither was copied into the page. The
  page writes microseconds out as "millionths of a second" instead, which a
  non-engineer can read anyway.
- No em dash appears in any file written for this page.
- No new colours were introduced, so every text pairing on the page is one the
  demo kit already measured for contrast.
