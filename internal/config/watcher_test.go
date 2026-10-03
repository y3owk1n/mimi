//nolint:testpackage // tests notifyChange, an unexported method exercising Watcher's private log/dispatch ordering
package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// newTestWatcher writes contents to a config file in dir and returns a
// Watcher over it, wired to onChange, plus the log entries notifyChange will
// produce on it.
func newTestWatcher(
	t *testing.T,
	dir, contents string,
	onChange func(),
) (*Watcher, *observer.ObservedLogs) {
	t.Helper()

	path := filepath.Join(dir, "config.toml")

	err := os.WriteFile(path, []byte(contents), 0o600)
	if err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	core, logs := observer.New(zapcore.DebugLevel)
	logger := zap.New(core).Sugar()

	return NewWatcher(path, onChange, logger), logs
}

// TestWatcher_NotifyChange_ReportsTheChangeAndNothingMore pins the fix for
// #107 and the split it grew into: the watcher says the file changed, and
// claims nothing about the reload. It does not read the file, so a config
// that will not parse reaches the daemon's one reload path — which loads it,
// applies it, and logs the single authoritative outcome line — instead of
// being reported here, in a second voice that cannot name the trigger.
func TestWatcher_NotifyChange_ReportsTheChangeAndNothingMore(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		contents string
	}{
		{name: "a config that parses", contents: "[settings]\nlog_level = \"info\"\n"},
		{name: "a config that does not parse", contents: "not valid toml [[["},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			called := 0

			onChange := func() { called++ }

			watcher, logs := newTestWatcher(t, t.TempDir(), testCase.contents, onChange)
			watcher.notifyChange()

			if called != 1 {
				t.Fatalf("onChange was called %d times, want 1", called)
			}

			entries := logs.All()
			if len(entries) != 1 {
				t.Fatalf("got %d log entries, want 1: %+v", len(entries), entries)
			}

			entry := entries[0]

			if entry.Message != "file changed" {
				t.Errorf("log message = %q, want %q", entry.Message, "file changed")
			}

			if entry.Level != zapcore.DebugLevel {
				t.Errorf(
					"watcher logged %q at %v, want Debug; the daemon's reloadConfig owns the reload outcome line",
					entry.Message,
					entry.Level,
				)
			}
		})
	}
}

// TestWatcher_Run_NoticesEverySaveStrategy pins that an edit reloads however
// the editor saves it. Before, deleting the file and writing it again ended
// the watch, and no later edit reloaded.
func TestWatcher_Run_NoticesEverySaveStrategy(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	changed := make(chan struct{}, 8)
	watcher, _ := newTestWatcher(t, dir, "# start\n", func() { changed <- struct{}{} })

	ctx := t.Context()

	go func() { _ = watcher.Run(ctx) }()

	// Run adds the watch when it starts, so wait a moment for it.
	time.Sleep(200 * time.Millisecond)

	write := func(contents string) {
		t.Helper()

		err := os.WriteFile(path, []byte(contents), 0o600)
		if err != nil {
			t.Fatalf("write config: %v", err)
		}
	}

	saves := []struct {
		name string
		save func()
	}{
		{"in place", func() { write("# in place\n") }},
		{"renamed over", func() {
			tmp := filepath.Join(dir, "config.toml.tmp")

			err := os.WriteFile(tmp, []byte("# renamed\n"), 0o600)
			if err != nil {
				t.Fatalf("write temp: %v", err)
			}

			err = os.Rename(tmp, path)
			if err != nil {
				t.Fatalf("rename: %v", err)
			}
		}},
		{"deleted and recreated", func() {
			err := os.Remove(path)
			if err != nil {
				t.Fatalf("remove: %v", err)
			}

			time.Sleep(100 * time.Millisecond)
			write("# recreated\n")
		}},
		{"edited after recreation", func() { write("# edited again\n") }},
	}

	for _, each := range saves {
		each.save()

		select {
		case <-changed:
		case <-time.After(3 * time.Second):
			t.Fatalf("save %q was not noticed", each.name)
		}

		// Let the debounce settle so the next save is its own change.
		time.Sleep(400 * time.Millisecond)

		for len(changed) > 0 {
			<-changed
		}
	}
}

// TestWatcher_Run_NoticesEditsThroughASymlinkedConfig pins that a config kept
// in a dotfiles directory and linked into place reloads when the file it
// links to is edited, since an editor that follows the link writes there.
func TestWatcher_Run_NoticesEditsThroughASymlinkedConfig(t *testing.T) {
	t.Parallel()

	dotfiles := t.TempDir()
	target := filepath.Join(dotfiles, "mimi.toml")

	err := os.WriteFile(target, []byte("# start\n"), 0o600)
	if err != nil {
		t.Fatalf("write target: %v", err)
	}

	link := filepath.Join(t.TempDir(), "config.toml")

	err = os.Symlink(target, link)
	if err != nil {
		t.Fatalf("symlink: %v", err)
	}

	changed := make(chan struct{}, 8)
	watcher := NewWatcher(link, func() { changed <- struct{}{} }, nil)

	ctx := t.Context()

	go func() { _ = watcher.Run(ctx) }()

	time.Sleep(200 * time.Millisecond)

	err = os.WriteFile(target, []byte("# edited\n"), 0o600)
	if err != nil {
		t.Fatalf("edit target: %v", err)
	}

	select {
	case <-changed:
	case <-time.After(3 * time.Second):
		t.Fatal("an edit to the linked file was not noticed")
	}
}
