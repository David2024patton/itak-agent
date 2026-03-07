package debug

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

// Level defines log severity.
type Level int

const (
	LevelError Level = iota
	LevelWarn
	LevelInfo
	LevelDebug
)

var levelNames = map[Level]string{
	LevelError: "ERROR",
	LevelWarn:  "WARN",
	LevelInfo:  "INFO",
	LevelDebug: "DEBUG",
}

var levelColors = map[Level]string{
	LevelError: "\033[31m",         // red
	LevelWarn:  "\033[33m",         // yellow
	LevelInfo:  "\033[36m",         // cyan
	LevelDebug: "\033[38;5;208m",   // orange (256-color)
}

const colorReset = "\033[0m"

// Logger is the iTaKAgent structured logger.
type Logger struct {
	mu       sync.Mutex
	level    Level
	output   io.Writer
	useColor bool
}

// Global logger instance.
var global = &Logger{
	level:    LevelInfo,
	output:   os.Stderr,
	useColor: true,
}

// SetLevel sets the global log level.
func SetLevel(l Level) {
	global.mu.Lock()
	defer global.mu.Unlock()
	global.level = l
}

// SetOutput sets the global log output writer.
func SetOutput(w io.Writer) {
	global.mu.Lock()
	defer global.mu.Unlock()
	global.output = w
}

// SetColor enables or disables color output.
func SetColor(enabled bool) {
	global.mu.Lock()
	defer global.mu.Unlock()
	global.useColor = enabled
}

// EnableDebug is a convenience to turn on DEBUG level.
func EnableDebug() {
	SetLevel(LevelDebug)
}

// log formats and writes a log message.
func logMsg(level Level, component string, format string, args ...interface{}) {
	global.mu.Lock()
	defer global.mu.Unlock()

	if level > global.level {
		return
	}

	timestamp := time.Now().Format("15:04:05.000")
	msg := fmt.Sprintf(format, args...)
	levelStr := levelNames[level]

	if global.useColor {
		color := levelColors[level]
		fmt.Fprintf(global.output, "%s%s %s [%s]%s %s\n",
			color, timestamp, levelStr, component, colorReset, msg)
	} else {
		fmt.Fprintf(global.output, "%s %s [%s] %s\n",
			timestamp, levelStr, component, msg)
	}
}

// Convenience logging functions with component tag.

func Error(component, format string, args ...interface{}) {
	logMsg(LevelError, component, format, args...)
}

func Warn(component, format string, args ...interface{}) {
	logMsg(LevelWarn, component, format, args...)
}

func Info(component, format string, args ...interface{}) {
	logMsg(LevelInfo, component, format, args...)
}

func Debug(component, format string, args ...interface{}) {
	logMsg(LevelDebug, component, format, args...)
}

// Separator prints a visual separator for debug readability.
func Separator(component string) {
	logMsg(LevelDebug, component, strings.Repeat("─", 60))
}

// JSON logs a JSON payload at DEBUG level, truncated for readability.
func JSON(component, label, payload string) {
	if global.level < LevelDebug {
		return
	}
	truncated := payload
	if len(truncated) > 500 {
		truncated = truncated[:500] + "...(truncated)"
	}
	logMsg(LevelDebug, component, "%s:\n%s", label, truncated)
}

// ParseLevel converts a string to a Level.
func ParseLevel(s string) Level {
	switch strings.ToLower(s) {
	case "debug":
		return LevelDebug
	case "info":
		return LevelInfo
	case "warn", "warning":
		return LevelWarn
	case "error":
		return LevelError
	default:
		return LevelInfo
	}
}
