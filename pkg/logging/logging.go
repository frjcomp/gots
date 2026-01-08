package logging

import (
    "log"
    "os"
    "strings"
    "sync/atomic"
)

type Level int32

const (
    Error Level = iota
    Warn
    Info
    Debug
)

var current Level = Info

func SetLevelFromString(s string) {
    switch strings.ToLower(strings.TrimSpace(s)) {
    case "debug":
        atomic.StoreInt32((*int32)(&current), int32(Debug))
    case "info", "":
        atomic.StoreInt32((*int32)(&current), int32(Info))
    case "warn", "warning":
        atomic.StoreInt32((*int32)(&current), int32(Warn))
    case "error", "err":
        atomic.StoreInt32((*int32)(&current), int32(Error))
    default:
        // Unknown -> keep default
    }
}

func SetQuiet(quiet bool) {
    if quiet {
        atomic.StoreInt32((*int32)(&current), int32(Error))
    }
}

func Enabled(l Level) bool {
    return l <= Level(atomic.LoadInt32((*int32)(&current)))
}

func Debugf(format string, args ...any) {
    if Enabled(Debug) {
        log.Printf(format, args...)
    }
}

func Infof(format string, args ...any) {
    if Enabled(Info) {
        log.Printf(format, args...)
    }
}

func Warnf(format string, args ...any) {
    if Enabled(Warn) {
        log.Printf(format, args...)
    }
}

func Errorf(format string, args ...any) {
    if Enabled(Error) {
        log.Printf(format, args...)
    }
}

// InitFromEnv allows setting level via env var GOTS_LOG_LEVEL and QUIET via GOTS_QUIET.
func InitFromEnv() {
    if v := os.Getenv("GOTS_LOG_LEVEL"); v != "" {
        SetLevelFromString(v)
    }
    if os.Getenv("GOTS_QUIET") == "1" || strings.EqualFold(os.Getenv("GOTS_QUIET"), "true") {
        SetQuiet(true)
    }
}
