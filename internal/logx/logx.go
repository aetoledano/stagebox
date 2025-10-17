package logx

import (
	"fmt"
	"io"
	"log"
	"os"
	"runtime/debug"
	"strings"
	"time"
)

type Level int

const (
	DEBUG Level = iota
	INFO
	WARN
	ERROR
)

var (
	level    = INFO
	file     *os.File
	writer   io.Writer
	logDebug = log.New(io.Discard, "DEBUG ", log.Ldate|log.Ltime|log.Lmicroseconds)
	logInfo  = log.New(io.Discard, "INFO  ", log.Ldate|log.Ltime|log.Lmicroseconds)
	logWarn  = log.New(io.Discard, "WARN  ", log.Ldate|log.Ltime|log.Lmicroseconds)
	logError = log.New(io.Discard, "ERROR ", log.Ldate|log.Ltime|log.Lmicroseconds)
)

// Init configures logging.
// - enableFileLogs=false  → logs go to stdout only (no file opened)
// - enableFileLogs=true   → logs go to 'path' (and also stdout if alsoStdout=true)
func Init(enableFileLogs bool, path string, alsoStdout bool) error {
	// Close any previous file
	if file != nil {
		_ = file.Close()
		file = nil
	}

	if !enableFileLogs {
		// stdout only
		writer = os.Stdout
		setOutputsLocked(writer)
		Info("logx: initialized → stdout only")
		return nil
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		// fall back to stdout only on error
		writer = os.Stdout
		setOutputsLocked(writer)
		Errorf("logx: failed to open log file: %v (using stdout)", err)
		return err
	}
	file = f

	if alsoStdout {
		writer = io.MultiWriter(file, os.Stdout)
	} else {
		writer = file
	}
	setOutputsLocked(writer)
	Infof("logx: initialized → file=%s alsoStdout=%v", path, alsoStdout)
	return nil
}

func setOutputsLocked(w io.Writer) {
	// respect current level
	logDebug.SetOutput(&levelFilter{w: w, min: DEBUG})
	logInfo.SetOutput(&levelFilter{w: w, min: INFO})
	logWarn.SetOutput(&levelFilter{w: w, min: WARN})
	logError.SetOutput(&levelFilter{w: w, min: ERROR})
}

// Close the log file (call on app shutdown)
func Close() error {
	if file != nil {
		err := file.Close()
		file = nil
		return err
	}
	return nil
}

// Rotate closes and reopens the same file path (simple manual rotation).
func Rotate() error {
	if file == nil {
		return fmt.Errorf("logx: not initialized")
	}
	path := file.Name()
	_ = file.Close()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	file = f
	setOutputsLocked(file)
	Info("logx: rotated")
	return nil
}

// SetLevel allows "debug","info","warn","error"
func SetLevel(s string) {
	switch strings.ToLower(s) {
	case "debug":
		level = DEBUG
	case "warn":
		level = WARN
	case "error":
		level = ERROR
	default:
		level = INFO
	}
}

// GetLevel returns current level as string
func GetLevel() string {
	switch level {
	case DEBUG:
		return "debug"
	case WARN:
		return "warn"
	case ERROR:
		return "error"
	default:
		return "info"
	}
}

// levelFilter ensures messages below the global level don’t print.
type levelFilter struct {
	w   io.Writer
	min Level
}

func (lf *levelFilter) Write(p []byte) (int, error) {
	cur := level
	if cur > lf.min {
		return len(p), nil // discard quietly
	}
	return lf.w.Write(p)
}

// Convenience logging funcs

func Debug(v ...any)            { logDebug.Output(2, sprint(v...)) }
func Info(v ...any)             { logInfo.Output(2, sprint(v...)) }
func Warn(v ...any)             { logWarn.Output(2, sprint(v...)) }
func Error(v ...any)            { logError.Output(2, sprint(v...)) }
func Debugf(f string, v ...any) { logDebug.Output(2, sprintf(f, v...)) }
func Infof(f string, v ...any)  { logInfo.Output(2, sprintf(f, v...)) }
func Warnf(f string, v ...any)  { logWarn.Output(2, sprintf(f, v...)) }
func Errorf(f string, v ...any) { logError.Output(2, sprintf(f, v...)) }

// RecoverPanic logs a panic + stack trace.
// Use as: defer logx.RecoverPanic("startup")
func RecoverPanic(context string) {
	if r := recover(); r != nil {
		logError.Output(2, sprintf("PANIC in %s: %v\n%s", context, r, string(debug.Stack())))
	}
}

// Helper to timestamp a one-line marker (useful around external launches)
func Mark(msg string) {
	Infof("— %s — %s", msg, time.Now().Format(time.RFC3339))
}

func sprint(v ...any) string            { return fmt.Sprint(v...) }
func sprintf(f string, v ...any) string { return fmt.Sprintf(f, v...) }
