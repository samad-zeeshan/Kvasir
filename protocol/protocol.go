// Package protocol turns text lines into commands and replies into bytes.
//
// It knows nothing about the store or the network, so RESP could replace
// it later without touching either.
package protocol

import (
	"errors"
	"fmt"
	"strings"
)

type Op int

const (
	OpGet Op = iota
	OpSet
	OpDel
)

type Command struct {
	Op    Op
	Key   string
	Value string
}

var ErrEmpty = errors.New("empty command")

// Parse reads one line like "SET name alice smith". Everything after the
// key is the value, so values can hold spaces without quoting rules.
func Parse(line string) (Command, error) {
	line = strings.TrimRight(line, "\r\n")
	if strings.TrimSpace(line) == "" {
		return Command{}, ErrEmpty
	}
	parts := strings.SplitN(line, " ", 3)
	verb := strings.ToUpper(parts[0])
	switch verb {
	case "GET", "DEL":
		if len(parts) != 2 || parts[1] == "" {
			return Command{}, fmt.Errorf("%s takes exactly one key", verb)
		}
		op := OpGet
		if verb == "DEL" {
			op = OpDel
		}
		return Command{Op: op, Key: parts[1]}, nil
	case "SET":
		if len(parts) != 3 || parts[1] == "" {
			return Command{}, errors.New("SET takes a key and a value")
		}
		return Command{Op: OpSet, Key: parts[1], Value: parts[2]}, nil
	default:
		return Command{}, fmt.Errorf("unknown command %q", parts[0])
	}
}

// Replies are single lines. A stored value that equals "(nil)" reads the
// same as a miss, the price of a human readable protocol.
func ReplyValue(v string) string { return v + "\n" }

func ReplyNil() string { return "(nil)\n" }

func ReplyOK() string { return "OK\n" }

func ReplyInt(n int) string { return fmt.Sprintf("%d\n", n) }

func ReplyError(err error) string { return "ERR " + err.Error() + "\n" }
