package hooks //nolint:testpackage // tests unexported hookOutputBuffer

import (
	"bytes"
	"strings"
	"testing"
)

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
