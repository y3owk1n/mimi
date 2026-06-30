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
