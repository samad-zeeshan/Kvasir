//go:build !windows

package main

import "time"

// Everywhere except Windows, time.Now's monotonic reading is already
// nanosecond grade, so there is nothing to work around.
var clockStart = time.Now()

func nanos() int64 { return int64(time.Since(clockStart)) }

func clockName() string { return "time.Now monotonic reading" }
