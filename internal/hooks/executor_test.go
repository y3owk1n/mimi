package hooks //nolint:testpackage // tests unexported hookOutputBuffer / baseEnv / eventEnv

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/y3owk1n/mimi/internal/config"
	"github.com/y3owk1n/mimi/internal/events"
)

const defaultShell = "/bin/sh"

func TestHookOutputBufferWritesWithinLimit(t *testing.T) {
	t.Parallel()

	buf := &hookOutputBuffer{limit: 16}

	written, err := buf.Write([]byte("hello"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if written != 5 {
		t.Fatalf("expected written=5, got %d", written)
	}

	if got := buf.Bytes(); !bytes.Equal(got, []byte("hello")) {
		t.Fatalf("expected %q, got %q", "hello", got)
	}
}

func TestHookOutputBufferTruncatesWritesPastLimit(t *testing.T) {
	t.Parallel()

	buf := &hookOutputBuffer{limit: 8}

	// First write fills most of the buffer.
	_, err := buf.Write([]byte("12345"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Second write of 10 bytes — only 3 should land, the rest is dropped.
	_, err = buf.Write([]byte("abcdefghij"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := buf.Bytes(); !bytes.Equal(got, []byte("12345abc")) {
		t.Fatalf("expected %q, got %q", "12345abc", got)
	}
}

func TestHookOutputBufferDropsWritesOnceFull(t *testing.T) {
	t.Parallel()

	buf := &hookOutputBuffer{limit: 4}

	_, err := buf.Write([]byte("abcd"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Buffer is full. Subsequent writes are silently dropped but Write
	// still reports the full input length (mirrors io.Discard) so the
	// child process doesn't see backpressure.
	written, err := buf.Write([]byte("xyz"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if written != 3 {
		t.Fatalf("expected written=3 (no backpressure), got %d", written)
	}

	if got := buf.Bytes(); !bytes.Equal(got, []byte("abcd")) {
		t.Fatalf("expected buffer unchanged %q, got %q", "abcd", got)
	}
}

func TestHookOutputBufferCapsAtLimit(t *testing.T) {
	t.Parallel()

	// Simulate a hook dumping 1 MiB of output — buffer should cap at limit.
	buf := &hookOutputBuffer{limit: maxHookOutputBytes}

	big := strings.Repeat("a", 1<<20) // 1 MiB

	_, err := buf.Write([]byte(big))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := len(buf.Bytes()); got != maxHookOutputBytes {
		t.Fatalf("expected buffer capped at %d, got %d", maxHookOutputBytes, got)
	}
}

// TestHookOutputBufferIntegration verifies that hookOutputBuffer actually
// caps the output of a real subprocess, not just synthetic Write calls.
func TestHookOutputBufferIntegration(t *testing.T) {
	t.Parallel()

	// Find a shell that's available on the test host. sh is the only
	// hard requirement since executor.go defaults to it.
	shell := defaultShell

	path, lookErr := exec.LookPath("sh")
	if lookErr == nil {
		shell = path
	}

	// Use /dev/zero piped through tr to produce an arbitrary 1 MiB of
	// output. head -c caps the producer at 1 MiB.
	const produced = 1 << 20

	cmd := exec.CommandContext(
		context.Background(),
		shell,
		"-c",
		"head -c "+strconv.Itoa(produced)+" /dev/zero | tr '\\0' a",
	)

	outBuf := &hookOutputBuffer{limit: maxHookOutputBytes}
	cmd.Stdout = outBuf
	cmd.Stderr = outBuf

	runErr := cmd.Run()
	if runErr != nil {
		t.Fatalf("subprocess failed: %v", runErr)
	}

	captured := outBuf.Bytes()
	if len(captured) != maxHookOutputBytes {
		t.Fatalf("expected %d bytes captured, got %d", maxHookOutputBytes, len(captured))
	}

	if !bytes.Equal(captured, bytes.Repeat([]byte("a"), maxHookOutputBytes)) {
		t.Fatal("captured output should be all 'a' bytes from tr")
	}
}

func TestEventEnvProducesAllMimiVars(t *testing.T) {
	t.Parallel()

	evt := events.Event{
		Kind:        events.WindowCreated,
		ID:          "abc-123",
		AppName:     "Safari",
		BundleID:    "com.apple.Safari",
		PID:         1234,
		WindowTitle: "My Window",
		At:          time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
		Extra: map[string]string{
			"foo": "bar",
		},
	}

	env := eventEnv(evt)

	want := []string{
		"mimi_EVENT=window_created",
		"mimi_EVENT_ID=abc-123",
		"mimi_APP_NAME=Safari",
		"mimi_BUNDLE_ID=com.apple.Safari",
		"mimi_PID=1234",
		"mimi_WINDOW_TITLE=My Window",
		"mimi_TIMESTAMP=2025-01-01T12:00:00Z",
		"mimi_FOO=bar",
	}

	envSet := make(map[string]struct{}, len(env))
	for _, e := range env {
		envSet[e] = struct{}{}
	}

	for _, w := range want {
		if _, ok := envSet[w]; !ok {
			t.Errorf("eventEnv missing %q\nfull env: %v", w, env)
		}
	}
}

func TestNewExecutorCapturesBaseEnv(t *testing.T) {
	// t.Setenv restores the previous value when the test ends, so this
	// is safe to run alongside other tests in the package.
	t.Setenv("MIMI_TEST_BASE_VAR_FOR_HOOKS", "captured-value-42")

	reg := NewRegistry()
	cfg := &config.SettingsConfig{
		HookShell:       "/bin/sh",
		HookTimeoutSecs: 5,
		MaxHookWorkers:  1,
	}
	exec := NewExecutor(reg, cfg, zap.NewNop().Sugar())

	if len(exec.baseEnv) == 0 {
		t.Fatal("baseEnv should be populated at construction")
	}

	want := "MIMI_TEST_BASE_VAR_FOR_HOOKS=captured-value-42"
	if slices.Contains(exec.baseEnv, want) {
		return
	}

	t.Errorf("baseEnv missing %q\nfull baseEnv length: %d", want, len(exec.baseEnv))
}

// TestExecutorMergesBaseAndEventEnv verifies end-to-end that a real hook
// subprocess can read both the base environment captured at executor
// construction and the per-event mimi_* environment variables.
func TestExecutorMergesBaseAndEventEnv(t *testing.T) {
	const (
		baseEnvKey = "MIMI_TEST_BASE_VAR_FOR_MERGE"
		baseEnvVal = "base-xyz-12345"
	)

	t.Setenv(baseEnvKey, baseEnvVal)

	outputFile := filepath.Join(t.TempDir(), "hook_output.txt")

	reg := NewRegistry()

	loadErr := reg.Reload(&config.Config{
		Hooks: config.HooksConfig{
			WindowCreated: []config.HookEntry{{
				Run: fmt.Sprintf(
					`printf '%%s|%s\n' "$%s" "$mimi_EVENT" > %s`,
					baseEnvKey, baseEnvKey, outputFile,
				),
			}},
		},
	})
	if loadErr != nil {
		t.Fatalf("registry reload: %v", loadErr)
	}

	cfg := &config.SettingsConfig{
		HookShell:       defaultShell,
		HookTimeoutSecs: 5,
		MaxHookWorkers:  1,
	}
	exec := NewExecutor(reg, cfg, zap.NewNop().Sugar())

	exec.Handle(events.Event{
		Kind:    events.WindowCreated,
		ID:      "merge-test",
		AppName: "TestApp",
		At:      time.Now(),
	})

	// Poll the output file — the hook runs in a worker so we need to
	// wait for it to finish.
	var content []byte

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		var readErr error

		content, readErr = os.ReadFile(outputFile) //nolint:gosec // test-controlled path
		if readErr == nil {
			break
		}

		time.Sleep(10 * time.Millisecond)
	}

	got := string(content)
	if !strings.Contains(got, baseEnvVal) {
		t.Errorf("hook output missing base env value %q\noutput: %q", baseEnvVal, got)
	}

	if !strings.Contains(got, string(events.WindowCreated)) {
		t.Errorf("hook output missing event env value %q\noutput: %q", events.WindowCreated, got)
	}
}

// TestHandle_LogsHookIndexNotCommand pins the contract that "hook skipped"
// and "hook matched" log the hook's index within its kind (enough to tell
// two hooks of the same kind apart), never the literal shell command from
// hook.Entry.Run. See AGENTS.md's logging contract and issue #112.
func TestHandle_LogsHookIndexNotCommand(t *testing.T) {
	t.Parallel()

	const secretCmd = "curl -H 'Authorization: Bearer sk-should-not-appear' https://example.internal"

	reg := NewRegistry()

	loadErr := reg.Reload(&config.Config{
		Hooks: config.HooksConfig{
			AppActivate: []config.HookEntry{
				{Run: "true"}, // index 0: matches
				{Run: secretCmd, App: "SomeOtherAppThatWontMatch"}, // index 1: skipped
			},
		},
	})
	if loadErr != nil {
		t.Fatalf("registry reload: %v", loadErr)
	}

	core, logs := observer.New(zapcore.DebugLevel)
	logger := zap.New(core).Sugar()

	cfg := &config.SettingsConfig{
		HookShell:       defaultShell,
		HookTimeoutSecs: 5,
		MaxHookWorkers:  1,
	}
	exec := NewExecutor(reg, cfg, logger)

	exec.Handle(events.Event{
		Kind:    events.AppActivate,
		ID:      "index-test",
		AppName: "TestApp",
	})

	matchedEntries := logs.FilterMessage("hook matched").All()
	if len(matchedEntries) != 1 {
		t.Fatalf("got %d \"hook matched\" entries, want 1", len(matchedEntries))
	}

	matchedFields := matchedEntries[0].ContextMap()

	if _, hasCmd := matchedFields["cmd"]; hasCmd {
		t.Errorf("\"hook matched\" entry logged \"cmd\": %+v", matchedFields)
	}

	gotIndex, hasIndex := matchedFields["index"]
	if !hasIndex {
		t.Fatalf("\"hook matched\" entry missing \"index\" field: %+v", matchedFields)
	}

	if gotIndex != int64(0) {
		t.Errorf("\"hook matched\" index = %v, want 0", gotIndex)
	}

	skippedEntries := logs.FilterMessage("hook skipped").All()
	if len(skippedEntries) != 1 {
		t.Fatalf("got %d \"hook skipped\" entries, want 1", len(skippedEntries))
	}

	skippedFields := skippedEntries[0].ContextMap()

	if _, hasCmd := skippedFields["cmd"]; hasCmd {
		t.Errorf("\"hook skipped\" entry logged \"cmd\": %+v", skippedFields)
	}

	gotSkippedIndex, hasSkippedIndex := skippedFields["index"]
	if !hasSkippedIndex {
		t.Fatalf("\"hook skipped\" entry missing \"index\" field: %+v", skippedFields)
	}

	if gotSkippedIndex != int64(1) {
		t.Errorf("\"hook skipped\" index = %v, want 1", gotSkippedIndex)
	}

	if gotReason, ok := skippedFields["reason"]; !ok || gotReason != "app filter mismatch" {
		t.Errorf("\"hook skipped\" reason = %v, want %q", gotReason, "app filter mismatch")
	}

	for _, entry := range logs.All() {
		if strings.Contains(entry.Message, secretCmd) {
			t.Fatalf("log entry leaked secret command in message: %q", entry.Message)
		}

		for key, val := range entry.ContextMap() {
			if s, ok := val.(string); ok && strings.Contains(s, secretCmd) {
				t.Fatalf("log field %q leaked secret command: %q", key, s)
			}
		}
	}
}

// assertNoLeak fails the test if any observed log entry carries the hook's
// command text or the hook's own output in its message or any string field.
func assertNoLeak(t *testing.T, logs *observer.ObservedLogs, secrets ...string) {
	t.Helper()

	for _, entry := range logs.All() {
		for _, secret := range secrets {
			if strings.Contains(entry.Message, secret) {
				t.Errorf("log message leaked %q: %q", secret, entry.Message)
			}

			for key, val := range entry.ContextMap() {
				s, ok := val.(string)
				if ok && strings.Contains(s, secret) {
					t.Errorf("log field %q leaked %q: %q", key, secret, s)
				}
			}
		}
	}
}

// TestRun_HookFailedLogsIndexWithoutCommandOrOutput pins the contract that the
// "hook failed" line — which fires at ERROR, i.e. without the user opting into
// anything — identifies the hook by its index within its kind and carries
// neither the command text nor the hook's stdout/stderr. See issue #117.
func TestRun_HookFailedLogsIndexWithoutCommandOrOutput(t *testing.T) {
	t.Parallel()

	const (
		secretCmd    = "printf 'sk-output-should-not-appear\\n'; exit 3"
		secretOutput = "sk-output-should-not-appear"
	)

	reg := NewRegistry()

	loadErr := reg.Reload(&config.Config{
		Hooks: config.HooksConfig{
			AppActivate: []config.HookEntry{
				{Run: testHookRun}, // index 0: succeeds
				{Run: secretCmd},   // index 1: fails, printing a secret
			},
		},
	})
	if loadErr != nil {
		t.Fatalf("registry reload: %v", loadErr)
	}

	core, logs := observer.New(zapcore.DebugLevel)

	cfg := &config.SettingsConfig{
		HookShell:       defaultShell,
		HookTimeoutSecs: 5,
		MaxHookWorkers:  1,
	}
	exec := NewExecutor(reg, cfg, zap.New(core).Sugar())

	exec.Handle(events.Event{
		Kind:    events.AppActivate,
		ID:      "failed-test",
		AppName: testAppName,
	})

	failedEntries := logs.FilterMessage("hook failed").All()
	if len(failedEntries) != 1 {
		t.Fatalf("got %d \"hook failed\" entries, want 1", len(failedEntries))
	}

	fields := failedEntries[0].ContextMap()

	if _, hasCmd := fields["cmd"]; hasCmd {
		t.Errorf("\"hook failed\" entry logged \"cmd\": %+v", fields)
	}

	if _, hasOutput := fields["output"]; hasOutput {
		t.Errorf("\"hook failed\" entry logged \"output\": %+v", fields)
	}

	gotIndex, hasIndex := fields["index"]
	if !hasIndex {
		t.Fatalf("\"hook failed\" entry missing \"index\" field: %+v", fields)
	}

	if gotIndex != int64(1) {
		t.Errorf("\"hook failed\" index = %v, want 1", gotIndex)
	}

	if _, hasExit := fields["exit"]; !hasExit {
		t.Errorf("\"hook failed\" entry missing \"exit\" field: %+v", fields)
	}

	assertNoLeak(t, logs, secretCmd, secretOutput)
}

// TestRun_HookTimedOutLogsIndexNotCommand pins that the "hook timed out" line
// — emitted at WARN, again without any opt-in — identifies the hook by index
// and never by its command text. See issue #117.
func TestRun_HookTimedOutLogsIndexNotCommand(t *testing.T) {
	t.Parallel()

	const secretCmd = "sleep 30 # sk-timeout-should-not-appear"

	reg := NewRegistry()

	loadErr := reg.Reload(&config.Config{
		Hooks: config.HooksConfig{
			AppActivate: []config.HookEntry{
				{Run: secretCmd, TimeoutSecs: 1},
			},
		},
	})
	if loadErr != nil {
		t.Fatalf("registry reload: %v", loadErr)
	}

	core, logs := observer.New(zapcore.DebugLevel)

	cfg := &config.SettingsConfig{
		HookShell:       defaultShell,
		HookTimeoutSecs: 30,
		MaxHookWorkers:  1,
	}
	exec := NewExecutor(reg, cfg, zap.New(core).Sugar())

	exec.Handle(events.Event{
		Kind:    events.AppActivate,
		ID:      "timeout-test",
		AppName: testAppName,
	})

	timedOutEntries := logs.FilterMessage("hook timed out").All()
	if len(timedOutEntries) != 1 {
		t.Fatalf("got %d \"hook timed out\" entries, want 1", len(timedOutEntries))
	}

	fields := timedOutEntries[0].ContextMap()

	if _, hasCmd := fields["cmd"]; hasCmd {
		t.Errorf("\"hook timed out\" entry logged \"cmd\": %+v", fields)
	}

	gotIndex, hasIndex := fields["index"]
	if !hasIndex {
		t.Fatalf("\"hook timed out\" entry missing \"index\" field: %+v", fields)
	}

	if gotIndex != int64(0) {
		t.Errorf("\"hook timed out\" index = %v, want 0", gotIndex)
	}

	if _, hasTimeout := fields["timeout"]; !hasTimeout {
		t.Errorf("\"hook timed out\" entry missing \"timeout\" field: %+v", fields)
	}

	assertNoLeak(t, logs, secretCmd)
}

// TestRun_HookOkLogsIndexNotCommandAndKeepsOutput pins both halves of the
// ruling on the debug path: "hook ok" identifies the hook by index rather
// than by its command text, but deliberately still carries the hook's own
// output, which is only ever emitted at debug. See issue #117.
func TestRun_HookOkLogsIndexNotCommandAndKeepsOutput(t *testing.T) {
	t.Parallel()

	const (
		secretCmd  = "printf 'hook-stdout-marker\\n' # sk-ok-should-not-appear"
		wantOutput = "hook-stdout-marker"
	)

	reg := NewRegistry()

	loadErr := reg.Reload(&config.Config{
		Hooks: config.HooksConfig{
			AppActivate: []config.HookEntry{
				{Run: testHookRun}, // index 0: succeeds silently
				{Run: secretCmd},   // index 1: succeeds with output
			},
		},
	})
	if loadErr != nil {
		t.Fatalf("registry reload: %v", loadErr)
	}

	core, logs := observer.New(zapcore.DebugLevel)

	cfg := &config.SettingsConfig{
		HookShell:       defaultShell,
		HookTimeoutSecs: 5,
		MaxHookWorkers:  1,
	}
	exec := NewExecutor(reg, cfg, zap.New(core).Sugar())

	exec.Handle(events.Event{
		Kind:    events.AppActivate,
		ID:      "ok-test",
		AppName: testAppName,
	})

	okEntries := logs.FilterMessage("hook ok").All()
	if len(okEntries) != 2 {
		t.Fatalf("got %d \"hook ok\" entries, want 2", len(okEntries))
	}

	for _, entry := range okEntries {
		if _, hasCmd := entry.ContextMap()["cmd"]; hasCmd {
			t.Errorf("\"hook ok\" entry logged \"cmd\": %+v", entry.ContextMap())
		}

		if _, hasIndex := entry.ContextMap()["index"]; !hasIndex {
			t.Errorf("\"hook ok\" entry missing \"index\" field: %+v", entry.ContextMap())
		}
	}

	fields := okEntries[1].ContextMap()

	if gotIndex := fields["index"]; gotIndex != int64(1) {
		t.Errorf("\"hook ok\" index = %v, want 1", gotIndex)
	}

	if _, hasElapsed := fields["elapsed"]; !hasElapsed {
		t.Errorf("\"hook ok\" entry missing \"elapsed\" field: %+v", fields)
	}

	// The debug path deliberately keeps the hook's output; only the
	// command text is removed.
	if gotOutput := fields["output"]; gotOutput != wantOutput {
		t.Errorf("\"hook ok\" output = %v, want %q", gotOutput, wantOutput)
	}

	assertNoLeak(t, logs, secretCmd)
}
