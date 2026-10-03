package native

import "C"

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Native log levels, matching MimiLogLevel in mimi_log.h.
const (
	nativeLogDebug = 0
	nativeLogInfo  = 1
	nativeLogWarn  = 2
)

//nolint:gochecknoglobals // native code has one logger per process
var nativeLogger atomic.Pointer[zap.SugaredLogger]

// SetLogger sends what native code logs to logger. Until it is called, as
// in a CLI action run without the daemon, warnings go to stderr and the
// rest is dropped.
func SetLogger(logger *zap.SugaredLogger) {
	nativeLogger.Store(logger)
}

// LogAnimationSteps logs at debug what a stepped animation cost: how many
// windows and frames, over how long, the slowest single write, and how many
// writes failed.
func LogAnimationSteps(windows, frames, failed int, elapsed, slowest time.Duration) {
	logger := nativeLogger.Load()
	if logger == nil {
		return
	}

	logger.Debugw("animation stepped",
		"windows", windows,
		"frames", frames,
		"failed", failed,
		"elapsed", elapsed.Round(time.Millisecond),
		"slowest", slowest.Round(100*time.Microsecond), //nolint:mnd // a tenth of a millisecond
	)
}

//export mimiNativeLogEnabled
func mimiNativeLogEnabled(level C.int) C.int {
	if nativeLogEnabled(nativeLogger.Load(), int(level)) {
		return 1
	}

	return 0
}

//export mimiNativeLog
func mimiNativeLog(level C.int, message, fields *C.char) {
	var fieldsJSON string
	if fields != nil {
		fieldsJSON = C.GoString(fields)
	}

	logNative(nativeLogger.Load(), os.Stderr, int(level), C.GoString(message), fieldsJSON)
}

func nativeLogEnabled(logger *zap.SugaredLogger, level int) bool {
	if logger == nil {
		return level >= nativeLogWarn
	}

	return logger.Level().Enabled(zapLevel(level))
}

// logNative writes one native line to logger, or to stderr when there is no
// logger yet. fieldsJSON is a JSON object whose keys become fields, in key
// order.
func logNative(logger *zap.SugaredLogger, stderr io.Writer, level int, message, fieldsJSON string) {
	keysAndValues := nativeFields(fieldsJSON)

	if logger != nil {
		logger.Logw(zapLevel(level), message, keysAndValues...)

		return
	}

	if level < nativeLogWarn {
		return
	}

	var line strings.Builder

	line.WriteString("mimi: ")
	line.WriteString(message)

	for i := 0; i+1 < len(keysAndValues); i += 2 {
		fmt.Fprintf(&line, " %v=%v", keysAndValues[i], keysAndValues[i+1])
	}

	_, _ = fmt.Fprintln(stderr, line.String())
}

func nativeFields(fieldsJSON string) []any {
	if fieldsJSON == "" {
		return nil
	}

	var fields map[string]any

	// Numbers stay as written, so an error code prints as -25200 and not
	// as a float.
	decoder := json.NewDecoder(strings.NewReader(fieldsJSON))
	decoder.UseNumber()

	err := decoder.Decode(&fields)
	if err != nil {
		return []any{"fields", fieldsJSON}
	}

	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}

	slices.Sort(keys)

	keysAndValues := make([]any, 0, 2*len(keys)) //nolint:mnd // a key and a value per field
	for _, key := range keys {
		keysAndValues = append(keysAndValues, key, fields[key])
	}

	return keysAndValues
}

func zapLevel(level int) zapcore.Level {
	switch level {
	case nativeLogDebug:
		return zapcore.DebugLevel
	case nativeLogInfo:
		return zapcore.InfoLevel
	default:
		return zapcore.WarnLevel
	}
}
