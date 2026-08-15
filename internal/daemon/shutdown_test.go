//nolint:testpackage // tests logEventDropCounts, an unexported function
package daemon

import (
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
