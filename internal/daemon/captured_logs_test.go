package daemon_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/y3owk1n/mimi/internal/daemon"
	derrors "github.com/y3owk1n/mimi/internal/errors"
)

// The environment entries the installed launchd plist carries the captured
// stream paths in, spelled out literally rather than taken from the daemon's
// own constants. internal/service writes these names into the plist and the
// daemon reads them back; nothing links the two but the names themselves, so
// the test on each side spells them and a rename fails one of them.
const (
	envCapturedStdout = "MIMI_CAPTURED_STDOUT"
	envCapturedStderr = "MIMI_CAPTURED_STDERR"
)

// seedCapturedLog writes a previous run's console output to name in dir and
// returns its path. Every case needs a file with something in it, because
// "left alone" and "emptied" are only distinguishable when there was something
// to lose.
func seedCapturedLog(t *testing.T, dir, name string) string {
	t.Helper()

	path := filepath.Join(dir, name)

	err := os.WriteFile(path, []byte("output from the previous run\n"), 0o644)
	if err != nil {
		t.Fatalf("seeding %s: %v", name, err)
	}

	return path
}

// unsetCapturedLogEnv removes both entries from the environment, for the cases
// that are about a daemon nothing told about a captured stream at all.
func unsetCapturedLogEnv(t *testing.T) {
	t.Helper()

	for _, name := range []string{envCapturedStdout, envCapturedStderr} {
		err := os.Unsetenv(name)
		if err != nil {
			t.Fatalf("unsetting %s: %v", name, err)
		}
	}
}

// assertSize fails unless the file at path is exactly want bytes long.
func assertSize(t *testing.T, path string, want int64) {
	t.Helper()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}

	if info.Size() != want {
		t.Errorf("%s is %d bytes, want %d", path, info.Size(), want)
	}
}

// TestTruncateCapturedLogs_EmptiesTheStreamsThePlistNames is the behavior the
// whole mechanism exists for: launchd holds those two files open from spawn and
// appends to them forever, and the daemon it respawns every ten seconds is the
// one that writes to them hardest. Truncating once at startup is the only
// rotation available to a process that does not own the descriptors, and it
// leaves each run's console output standing alone.
func TestTruncateCapturedLogs_EmptiesTheStreamsThePlistNames(t *testing.T) {
	dir := t.TempDir()
	stdout := seedCapturedLog(t, dir, "mimi.out.log")
	stderr := seedCapturedLog(t, dir, "mimi.err.log")

	t.Setenv(envCapturedStdout, stdout)
	t.Setenv(envCapturedStderr, stderr)

	truncated, err := daemon.TruncateCapturedLogs()
	if err != nil {
		t.Fatalf("TruncateCapturedLogs() = %v, want nil", err)
	}

	if truncated != 2 {
		t.Errorf("TruncateCapturedLogs() truncated %d, want 2", truncated)
	}

	assertSize(t, stdout, 0)
	assertSize(t, stderr, 0)
}

// TestTruncateCapturedLogs_LeavesEverythingAloneWithoutTheEnvironment pins the
// guard on all of it. A daemon started by hand — `mimi start`, with a terminal
// on stdout — is told about no captured streams, and must empty nothing: the
// files beside settings.log_file still hold the previous launchd run's crash
// log, which is the single artifact this whole mechanism exists to preserve.
func TestTruncateCapturedLogs_LeavesEverythingAloneWithoutTheEnvironment(t *testing.T) {
	dir := t.TempDir()
	stdout := seedCapturedLog(t, dir, "mimi.out.log")
	stderr := seedCapturedLog(t, dir, "mimi.err.log")

	seeded, err := os.Stat(stdout)
	if err != nil {
		t.Fatalf("stat %s: %v", stdout, err)
	}

	// Set by nothing: the plist is the only thing that sets these, and there is
	// no plist in front of a hand-started daemon. The Setenv pair is what
	// registers the restore; the unset is the state under test.
	t.Setenv(envCapturedStdout, "")
	t.Setenv(envCapturedStderr, "")
	unsetCapturedLogEnv(t)

	truncated, err := daemon.TruncateCapturedLogs()
	if err != nil {
		t.Fatalf("TruncateCapturedLogs() = %v, want nil", err)
	}

	if truncated != 0 {
		t.Errorf("TruncateCapturedLogs() truncated %d, want 0", truncated)
	}

	assertSize(t, stdout, seeded.Size())
	assertSize(t, stderr, seeded.Size())
}

// TestTruncateCapturedLogs_TruncatesOnlyTheStreamsItWasGiven covers the plist
// that names one stream and not the other. Half an environment is half the
// work, not none of it and not all of it.
func TestTruncateCapturedLogs_TruncatesOnlyTheStreamsItWasGiven(t *testing.T) {
	dir := t.TempDir()
	stdout := seedCapturedLog(t, dir, "mimi.out.log")
	stderr := seedCapturedLog(t, dir, "mimi.err.log")

	seeded, err := os.Stat(stderr)
	if err != nil {
		t.Fatalf("stat %s: %v", stderr, err)
	}

	t.Setenv(envCapturedStdout, stdout)
	t.Setenv(envCapturedStderr, "")

	truncated, err := daemon.TruncateCapturedLogs()
	if err != nil {
		t.Fatalf("TruncateCapturedLogs() = %v, want nil", err)
	}

	if truncated != 1 {
		t.Errorf("TruncateCapturedLogs() truncated %d, want 1", truncated)
	}

	assertSize(t, stdout, 0)
	assertSize(t, stderr, seeded.Size())
}

// TestTruncateCapturedLogs_IgnoresAStreamThatIsNotThereYet covers the first
// run after an install: launchd creates both files when it spawns the daemon,
// so a missing one means nobody has written to it, and there is nothing to
// empty. That is not a failure to report.
func TestTruncateCapturedLogs_IgnoresAStreamThatIsNotThereYet(t *testing.T) {
	dir := t.TempDir()
	stderr := seedCapturedLog(t, dir, "mimi.err.log")

	t.Setenv(envCapturedStdout, filepath.Join(dir, "never-written.out.log"))
	t.Setenv(envCapturedStderr, stderr)

	truncated, err := daemon.TruncateCapturedLogs()
	if err != nil {
		t.Fatalf("TruncateCapturedLogs() = %v, want nil", err)
	}

	if truncated != 1 {
		t.Errorf("TruncateCapturedLogs() truncated %d, want 1", truncated)
	}

	assertSize(t, stderr, 0)
}

// TestTruncateCapturedLogs_ReportsAStreamItCouldNotEmpty pins that a stream
// this cannot touch is said out loud and costs the other one nothing. The
// daemon starts either way — a console log that keeps growing is worth
// complaining about and not worth refusing to run over.
func TestTruncateCapturedLogs_ReportsAStreamItCouldNotEmpty(t *testing.T) {
	dir := t.TempDir()
	stderr := seedCapturedLog(t, dir, "mimi.err.log")

	// A directory where a captured stream should be: present, and nothing this
	// can empty.
	blocked := filepath.Join(dir, "mimi.out.log")

	err := os.Mkdir(blocked, 0o755)
	if err != nil {
		t.Fatalf("seeding blocking directory: %v", err)
	}

	t.Setenv(envCapturedStdout, blocked)
	t.Setenv(envCapturedStderr, stderr)

	truncated, err := daemon.TruncateCapturedLogs()
	if err == nil {
		t.Fatal("TruncateCapturedLogs() = nil, want the failure it hit")
	}

	if derrors.GetCode(err) != derrors.CodeLoggingFailed {
		t.Errorf(
			"TruncateCapturedLogs() code = %v, want %v",
			derrors.GetCode(err),
			derrors.CodeLoggingFailed,
		)
	}

	if truncated != 1 {
		t.Errorf("TruncateCapturedLogs() truncated %d, want the other stream done", truncated)
	}

	assertSize(t, stderr, 0)
}
