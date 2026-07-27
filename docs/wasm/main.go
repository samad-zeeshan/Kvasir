//go:build js && wasm

// Command kvstore-wasm is a thin bridge that lets a browser page drive the
// real kvstore packages. It exists only so the demo site under docs/ can run
// the repository's own code instead of a mock of it.
//
// The build constraint above is load bearing. Without it, "go build ./..."
// and "go test ./..." on a normal desktop would try to compile syscall/js
// and fail. With it, this directory is invisible to every build except
// GOOS=js GOARCH=wasm.
//
// What is real here: protocol.Parse does the parsing, the protocol package's
// own reply functions produce every reply byte, and store.MemoryStore holds
// the data, the same type cmd/kvstore serves over TCP.
//
// What is not here: the server package. It needs a real TCP listener and a
// browser page has no sockets, so its three way dispatch (the unexported
// (*Server).execute in server/server.go) is mirrored below and nothing else
// is. Keep this file boring.
package main

import (
	"syscall/js"

	"kvstore/protocol"
	"kvstore/store"
)

// st is the live map. One page, one store, exactly like one server process.
var st store.Store = store.NewMemoryStore()

// seen is a list of key names the page has mentioned, kept only so the
// drawer has something to ask about. It is never a source of truth: every
// value the page displays is read back out of st through the real Get.
var seen []string

const seenCap = 64

func remember(key string) {
	for _, k := range seen {
		if k == key {
			return
		}
	}
	if len(seen) >= seenCap {
		return
	}
	seen = append(seen, key)
}

// execute mirrors (*server.Server).execute in server/server.go, which is
// unexported and unreachable from here. It is the only logic in this file.
func execute(cmd protocol.Command) string {
	switch cmd.Op {
	case protocol.OpGet:
		v, ok := st.Get(cmd.Key)
		if !ok {
			return protocol.ReplyNil()
		}
		return protocol.ReplyValue(v)
	case protocol.OpSet:
		st.Set(cmd.Key, cmd.Value)
		return protocol.ReplyOK()
	case protocol.OpDel:
		if st.Delete(cmd.Key) {
			return protocol.ReplyInt(1)
		}
		return protocol.ReplyInt(0)
	}
	return protocol.ReplyValue("ERR unhandled op")
}

// rows re-reads every remembered key straight out of the store, so what the
// page draws is what the map holds, not a copy kept alongside it.
func rows() []any {
	out := make([]any, 0, len(seen))
	for _, k := range seen {
		if v, ok := st.Get(k); ok {
			out = append(out, map[string]any{"key": k, "value": v})
		}
	}
	return out
}

// eval takes one line of the text protocol and returns what the page needs
// to draw: the reply exactly as the TCP server would write it, the current
// contents of the map, and which key the line touched.
func eval(line string) map[string]any {
	res := map[string]any{
		"line":    line,
		"touched": "",
		"verb":    "",
	}

	cmd, err := protocol.Parse(line)
	if err != nil {
		res["reply"] = protocol.ReplyError(err)
		res["ok"] = false
		res["rows"] = rows()
		return res
	}

	switch cmd.Op {
	case protocol.OpGet:
		res["verb"] = "GET"
	case protocol.OpSet:
		res["verb"] = "SET"
		remember(cmd.Key)
	case protocol.OpDel:
		res["verb"] = "DEL"
	}

	res["reply"] = execute(cmd)
	res["ok"] = true
	res["touched"] = cmd.Key
	res["rows"] = rows()
	return res
}

func main() {
	js.Global().Set("kvstoreEval", js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) < 1 {
			return js.ValueOf(map[string]any{
				"reply": "ERR nothing sent\n",
				"ok":    false,
				"rows":  rows(),
			})
		}
		return js.ValueOf(eval(args[0].String()))
	}))

	js.Global().Set("kvstoreReady", js.ValueOf(true))
	if fn := js.Global().Get("kvstoreOnReady"); fn.Type() == js.TypeFunction {
		fn.Invoke()
	}

	// Park forever. Exiting main would tear down the exported function.
	select {}
}
