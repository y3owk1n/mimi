//nolint:testpackage // tests logEventDropCounts, an unexported function
package daemon

import (
	"fmt"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// TestLogEventDropCounts_LogsBothCountersAsDistinctFields pins the contract
// that the daemon's only observability point for these two counters logs
// both the native-side and bus-side drop totals as separate fields, so a
// maintainer can tell which layer is under backpressure.
func TestLogEventDropCounts_LogsBothCountersAsDistinctFields(t *testing.T) {
	t.Parallel()

	core, logs := observer.New(zapcore.InfoLevel)
	logger := zap.New(core).Sugar()

	const (
		nativeDropped = int64(3)
		busDropped    = int64(7)
	)

	logEventDropCounts(nativeDropped, busDropped, logger)

	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("got %d log entries, want 1", len(entries))
	}

	entry := entries[0]

	gotNative, hasNative := entry.ContextMap()["native_dropped"]
	if !hasNative {
		t.Fatalf("log entry missing native_dropped field: %+v", entry.ContextMap())
	}

	if gotNative != nativeDropped {
		t.Errorf("native_dropped = %v, want %d", gotNative, nativeDropped)
	}

	gotBus, hasBus := entry.ContextMap()["bus_dropped"]
	if !hasBus {
		t.Fatalf("log entry missing bus_dropped field: %+v", entry.ContextMap())
	}

	if gotBus != busDropped {
		t.Errorf("bus_dropped = %v, want %d", gotBus, busDropped)
	}
}

func TestLogEventDropCounts_WarnsWhenAnythingWasDropped(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name        string
		native, bus int64
		wantLevel   zapcore.Level
	}{
		{"nothing dropped", 0, 0, zapcore.InfoLevel},
		{"native dropped", 1, 0, zapcore.WarnLevel},
		{"bus dropped", 0, 2, zapcore.WarnLevel},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			core, logs := observer.New(zapcore.DebugLevel)
			logEventDropCounts(testCase.native, testCase.bus, zap.New(core).Sugar())

			if entries := logs.All(); len(entries) != 1 || entries[0].Level != testCase.wantLevel {
				t.Errorf("entries = %+v, want one at %v", entries, testCase.wantLevel)
			}
		})
	}
}

func TestDropLogger_WarnsOnceThenLogsAtDebug(t *testing.T) {
	t.Parallel()

	core, logs := observer.New(zapcore.DebugLevel)
	onDrop := dropLogger(zap.New(core).Sugar())

	onDrop("window_moved", 256)
	onDrop("window_moved", 256)
	onDrop("window_focused", 256)

	entries := logs.All()
	if len(entries) != 3 {
		t.Fatalf("got %d entries, want 3", len(entries))
	}

	if entries[0].Level != zapcore.WarnLevel || entries[1].Level != zapcore.DebugLevel ||
		entries[2].Level != zapcore.DebugLevel {
		t.Errorf("levels = [%v, %v, %v], want a warning first and debug after",
			entries[0].Level, entries[1].Level, entries[2].Level)
	}

	if fmt.Sprint(entries[0].ContextMap()["kind"]) != "window_moved" ||
		entries[0].ContextMap()["buffer"] != int64(256) {
		t.Errorf("first drop fields = %v, want kind and buffer", entries[0].ContextMap())
	}
}
