package log

import (
	"fmt"
	"os"
	"sync/atomic"
)

var level atomic.Int32

type Level int32

const (
	LevelDebug   Level = 2
	LevelVerbose Level = 1
	LevelError   Level = 0
)

// SetLevel sets current log level.
func SetLevel(l Level) {
	if l < LevelError || l > LevelDebug {
		panic(fmt.Errorf("invalid log level: %#v", l))
	}
	level.Store(int32(l))
}

// Verbosef prints on verbose level.
func Verbosef(format string, a ...any) {
	if level.Load() < int32(LevelVerbose) {
		return
	}
	fmt.Fprintf(os.Stderr, format, a...)
}

// Debugf prints on debug level.
func Debugf(format string, a ...any) {
	if level.Load() < int32(LevelDebug) {
		return
	}
	fmt.Fprintf(os.Stderr, format, a...)
}

// Errorf prints on error level.
func Errorf(format string, a ...any) {
	fmt.Fprintf(os.Stderr, format, a...)
}
