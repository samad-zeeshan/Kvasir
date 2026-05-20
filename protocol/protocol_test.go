// Table driven tests for command parsing and reply encoding.
package protocol

import (
	"errors"
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		line    string
		want    Command
		wantErr bool
	}{
		{"get", "GET name", Command{Op: OpGet, Key: "name"}, false},
		{"set", "SET name alice", Command{Op: OpSet, Key: "name", Value: "alice"}, false},
		{"del", "DEL name", Command{Op: OpDel, Key: "name"}, false},
		{"lowercase verb", "get name", Command{Op: OpGet, Key: "name"}, false},
		{"value with spaces", "SET name alice smith jr", Command{Op: OpSet, Key: "name", Value: "alice smith jr"}, false},
		{"crlf trimmed", "GET name\r\n", Command{Op: OpGet, Key: "name"}, false},
		{"empty line", "", Command{}, true},
		{"blank line", "   ", Command{}, true},
		{"unknown verb", "PING", Command{}, true},
		{"get no key", "GET", Command{}, true},
		{"get extra arg", "GET a b", Command{}, true},
		{"set no value", "SET key", Command{}, true},
		{"del no key", "DEL", Command{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.line)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Parse(%q) succeeded, want error", tt.line)
				}
				return
			}
			if err != nil {
				t.Fatalf("Parse(%q) failed: %v", tt.line, err)
			}
			if got != tt.want {
				t.Fatalf("Parse(%q) = %+v, want %+v", tt.line, got, tt.want)
			}
		})
	}
}

func TestReplies(t *testing.T) {
	if got := ReplyValue("alice"); got != "alice\n" {
		t.Errorf("ReplyValue = %q", got)
	}
	if got := ReplyNil(); got != "(nil)\n" {
		t.Errorf("ReplyNil = %q", got)
	}
	if got := ReplyOK(); got != "OK\n" {
		t.Errorf("ReplyOK = %q", got)
	}
	if got := ReplyInt(1); got != "1\n" {
		t.Errorf("ReplyInt = %q", got)
	}
	if got := ReplyError(errors.New("boom")); got != "ERR boom\n" {
		t.Errorf("ReplyError = %q", got)
	}
}
