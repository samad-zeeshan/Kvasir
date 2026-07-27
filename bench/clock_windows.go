//go:build windows

// Windows needs an explicit high resolution clock. Go's time.Now on this
// platform reads the interrupt time page, which ticks roughly every half
// a millisecond, so a 20us round trip measures as either zero or 534us
// and every percentile below p99 prints as 0. QueryPerformanceCounter is
// the 100ns counter the platform provides for exactly this, reached
// through syscall so the repo stays dependency free.
package main

import (
	"fmt"
	"syscall"
	"unsafe"
)

var (
	kernel32                      = syscall.NewLazyDLL("kernel32.dll")
	procQueryPerformanceCounter   = kernel32.NewProc("QueryPerformanceCounter")
	procQueryPerformanceFrequency = kernel32.NewProc("QueryPerformanceFrequency")

	qpcFreq int64 // ticks per second, fixed at boot
)

func init() {
	procQueryPerformanceFrequency.Call(uintptr(unsafe.Pointer(&qpcFreq)))
	if qpcFreq == 0 {
		fail("QueryPerformanceFrequency reported no usable clock")
	}
}

func nanos() int64 {
	var c int64
	procQueryPerformanceCounter.Call(uintptr(unsafe.Pointer(&c)))
	// Split the divide from the multiply. Scaling the raw count by 1e9
	// first overflows int64 once the box has been up a few minutes.
	return (c/qpcFreq)*1e9 + (c%qpcFreq)*1e9/qpcFreq
}

func clockName() string {
	return fmt.Sprintf("QueryPerformanceCounter at %d Hz", qpcFreq)
}
