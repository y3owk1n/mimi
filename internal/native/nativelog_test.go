//nolint:testpackage // the bridge is reached from C, so its Go side has no exported entry
package native

import (
	"bytes"
	"fmt"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestLogNative_FieldsBecomeLogFieldsAtTheNativeLevel(t *testing.T) {
	t.Parallel()

	core, logs := observer.New(zapcore.DebugLevel)

	logNative(zap.New(core).Sugar(), &bytes.Buffer{}, nativeLogWarn, "AX write failed",
		`{"pid":42,"attribute":"AXSize"}`)

	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}

	entry := entries[0]
	if entry.Level != zapcore.WarnLevel || entry.Message != "AX write failed" {
		t.Errorf("entry = %v %q, want warn %q", entry.Level, entry.Message, "AX write failed")
	}

	fields := entry.ContextMap()
	if fmt.Sprint(fields["pid"]) != "42" || fields["attribute"] != "AXSize" {
		t.Errorf("fields = %v, want pid=42 attribute=AXSize", fields)
	}
}

func TestLogNative_WithoutALoggerOnlyWarningsReachStderr(t *testing.T) {
	t.Parallel()

	var stderr bytes.Buffer

	logNative(nil, &stderr, nativeLogDebug, "window list unavailable", "")
	logNative(
		nil,
		&stderr,
		nativeLogWarn,
		"permission reset failed",
		`{"status":1,"service":"Accessibility"}`,
	)

	want := "mimi: permission reset failed service=Accessibility status=1\n"
	if stderr.String() != want {
		t.Errorf("stderr = %q, want %q", stderr.String(), want)
	}
}

func TestNativeLogEnabled_FollowsTheLoggerLevel(t *testing.T) {
	t.Parallel()

	core, _ := observer.New(zapcore.InfoLevel)
	logger := zap.New(core).Sugar()

	if nativeLogEnabled(logger, nativeLogDebug) {
		t.Error("debug enabled on an info logger")
	}

	if !nativeLogEnabled(logger, nativeLogInfo) {
		t.Error("info disabled on an info logger")
	}

	if nativeLogEnabled(nil, nativeLogInfo) || !nativeLogEnabled(nil, nativeLogWarn) {
		t.Error("without a logger, only warnings should be enabled")
	}
}
