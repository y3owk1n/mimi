package daemon

import (
	"errors"
	"os"

	derrors "github.com/y3owk1n/mimi/internal/errors"
)

// The environment entries the installed launchd plist carries the daemon's
// captured stdout and stderr in.
//
// They are set by the plist `mimi services install` renders and by the agents
// the Nix modules render, and by nothing else — every one of them a launchd
// job of mimi's. The daemon deliberately does not ask internal/service to
// derive these paths for it: that package is the install-time surface, and a
// daemon that derived them itself would truncate files it was never told it
// owns — every hand-started `mimi start` would empty the previous launchd
// run's crash log, which is the one artifact the captured streams exist to
// preserve. Learning them from the environment is what makes "launchd started
// me" the condition, with no way to get it wrong.
//
// internal/service spells these same two names in its plist template. Nothing
// but the names holds the two sides together, so the test on each side spells
// the literals and a rename fails a test there. nix/darwin.nix and
// nix/home.nix spell them a third time, out of reach of any Go test — a rename
// has to be carried into both by hand.
const (
	envCapturedStdout = "MIMI_CAPTURED_STDOUT"
	envCapturedStderr = "MIMI_CAPTURED_STDERR"
)

// TruncateCapturedLogs empties the console streams the installed service
// captures, and reports how many it emptied.
//
// launchd opens those two files when it spawns the daemon and appends to them
// for as long as the machine stays logged in, with no rotation of any kind —
// and the daemon they exist to diagnose, one crash-looping under KeepAlive, is
// the one that writes to them fastest and for longest. The daemon cannot rotate
// files whose descriptors launchd holds, but it can empty them once at startup,
// which bounds them at one run's output and leaves that run's console output
// standing alone.
//
// It must run before anything is written to those streams, and so before the
// logger exists — the count and any failure are returned for the caller to log
// once it does. Running first is also what keeps the truncation safe: the
// process has not yet written a byte through the descriptors launchd handed it,
// so emptying the files behind them cannot leave a write landing past a new
// end of file.
//
// A daemon told about no captured streams empties nothing: an entry that is
// unset or empty is skipped, and so is a file that is not there yet, which is
// simply a stream nobody has written to. Anything else is reported, and never
// stops the other stream from being emptied — a console log that goes on
// growing is worth saying out loud and not worth refusing to start over.
func TruncateCapturedLogs() (int, error) {
	var (
		truncated int
		failures  []error
	)

	for _, name := range []string{envCapturedStdout, envCapturedStderr} {
		path := os.Getenv(name)
		if path == "" {
			continue
		}

		err := os.Truncate(path, 0)

		switch {
		case err == nil:
			truncated++
		case os.IsNotExist(err):
			// launchd creates the file when it spawns the job, so a missing one
			// is a stream that has never been written to.
		default:
			failures = append(failures, err)
		}
	}

	if len(failures) > 0 {
		return truncated, derrors.Wrapf(
			errors.Join(failures...),
			derrors.CodeLoggingFailed,
			"truncating the captured console logs",
		)
	}

	return truncated, nil
}
