window.DEMO_DATA = window.DEMO_DATA || {};
window.DEMO_DATA["kvstore"] = {
  "name": "kvstore",
  "what_runs_in_the_page": "The store and protocol packages from this repository, compiled to WebAssembly (a way browsers run compiled code). The server package is not included, because a browser page has no network sockets.",
  "built_on": "2026-07-27",
  "commit": "fd71c7f",
  "go_version": "go1.26.4",
  "wasm_bytes_raw": 2692397,
  "wasm_bytes_gzipped": 770905,
  "wasm_gzip_note": "gzip -9. GitHub Pages compresses on the way out, so the gzipped figure is what a reader downloads.",
  "captured_session": {
    "captured_on": "2026-07-27",
    "how": "The real server was started with 'go run ./cmd/kvstore -addr 127.0.0.1:7911' and driven over a plain network connection by a scripted client. Every reply below is the exact line the server wrote back.",
    "purpose": "Shown in place of the live prompt when the page is opened from a local file, where browsers refuse to load the compiled code.",
    "lines": [
      {
        "command": "SET name alice",
        "reply": "OK"
      },
      {
        "command": "GET name",
        "reply": "alice"
      },
      {
        "command": "SET greeting hello there friend",
        "reply": "OK"
      },
      {
        "command": "GET greeting",
        "reply": "hello there friend"
      },
      {
        "command": "GET nothing",
        "reply": "(nil)"
      },
      {
        "command": "DEL name",
        "reply": "1"
      },
      {
        "command": "GET name",
        "reply": "(nil)"
      },
      {
        "command": "PING",
        "reply": "ERR unknown command \"PING\""
      },
      {
        "command": "GET",
        "reply": "ERR GET takes exactly one key"
      }
    ],
    "final_state": [
      {
        "key": "greeting",
        "value": "hello there friend"
      }
    ],
    "final_state_note": "Read straight off the captured replies above. GET greeting returned the value, and GET name returned (nil) once DEL had removed it, so greeting is the only label left."
  },
  "measurements": {
    "source_file": "bench/RESULTS.md",
    "measured_on": "2026-07-27",
    "machine": "AMD Ryzen 5 7600X3D, 6 cores and 12 threads, Windows 11, Go 1.26.4",
    "method": "Client and server in separate processes on one machine, talking over that machine's own loopback network. Each client waits for its answer before asking again. Every cell was run 7 times and the middle result is reported, with the spread between the fastest and slowest run beside it.",
    "honest_configuration_note": "bench/RESULTS.md names the cross-process table the honest configuration and the one to quote. The in-process table in that file reports higher numbers and is not used here.",
    "cross_process_32_clients": [
      {
        "workload": "all reads",
        "ops_per_sec": 107783,
        "spread_percent": 10.1,
        "p50_us": 234,
        "p90_us": 417,
        "p99_us": 880
      },
      {
        "workload": "9 reads to 1 write",
        "ops_per_sec": 142851,
        "spread_percent": 10.2,
        "p50_us": 198,
        "p90_us": 305,
        "p99_us": 514
      },
      {
        "workload": "all writes",
        "ops_per_sec": 126871,
        "spread_percent": 20.3,
        "p50_us": 215,
        "p90_us": 355,
        "p99_us": 638
      }
    ],
    "store_alone": {
      "source_file": "bench/RESULTS.md, section 'The store and its lock, on its own'",
      "command": "go test -bench=. -benchmem -benchtime=3s -count=3 ./store",
      "read_ns_per_op": 26.93,
      "write_ns_per_op": 54.06,
      "allocations_per_op": 0,
      "note": "Both benchmarks run on all 12 threads at once against a single key, the worst contention the type can see."
    },
    "budget": {
      "source_file": "bench/RESULTS.md, finding 1",
      "per_operation_budget_us": 8.6,
      "spent_in_store_ns": 27,
      "share_percent": 0.4,
      "note": "At the measured cross-process read rate the server has 8.6 millionths of a second per operation and spends 27 billionths of it in the storage code."
    }
  },
  "tests": {
    "source": "go test -v -count=1 ./... on 2026-07-27",
    "test_functions": 15,
    "results_reported": 28,
    "failures": 0,
    "files": [
      "store/store_test.go",
      "server/server_test.go",
      "protocol/protocol_test.go",
      "cmd/kvstore-web/main_test.go"
    ]
  }
};
