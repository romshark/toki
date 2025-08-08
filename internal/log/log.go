// Package log is a thin wrapper around Go's standard log/slog.
package log

import (
	"fmt"
	"go/token"
	"io"
	"log/slog"
	"os"
	"sync/atomic"
)

type Mode int8

const (
	ModeQuiet   Mode = 0
	ModeRegular Mode = 1
	ModeVerbose Mode = 2
)

func init() { SetMode(ModeRegular) }

var mode atomic.Int32

func SetMode(m Mode) { mode.Store(int32(m)) }

func SetWriter(stderr io.Writer, jsonMode bool) {
	var l *slog.Logger
	if jsonMode {
		l = slog.New(slog.NewJSONHandler(stderr, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		}))
	} else {
		l = slog.New(newColorHandler(stderr, slog.LevelInfo))
	}
	logger.Store(l)
}

var logger atomic.Value

func getLogger() *slog.Logger {
	l, ok := logger.Load().(*slog.Logger)
	if ok {
		return l
	}
	SetWriter(os.Stderr, false)
	return logger.Load().(*slog.Logger)
}

func Verbose(msg string, attrs ...any) {
	if Mode(mode.Load()) >= ModeVerbose {
		getLogger().Info(msg, attrs...)
	}
}

func Info(msg string, attrs ...any) {
	if Mode(mode.Load()) >= ModeRegular {
		getLogger().Info(msg, attrs...)
	}
}

func Warn(msg string, attrs ...any) {
	if Mode(mode.Load()) >= ModeRegular {
		getLogger().Warn(msg, attrs...)
	}
}

func Error(msg string, err error, attrs ...any) {
	l := getLogger()
	if err == nil {
		l.Error(msg, attrs...)
		return
	}
	attrs = append([]any{slog.Any("error", err.Error())}, attrs...)
	l.Error(msg, attrs...)
}

func FmtPos(pos token.Position) string {
	return fmt.Sprintf("%s:%d:%d", pos.Filename, pos.Line, pos.Column)
}
