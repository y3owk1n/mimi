package service

import (
	"context"
	"errors"
	"os/exec"
)

// launcher is the seam between [Service] and launchctl. The production
// implementation shells out; tests fake it so install/uninstall/start/stop/
// status can be exercised without a real launchctl underneath them.
type launcher interface {
	// list reports whether label is currently loaded, and separately whether
	// it could be asked at all. Its stdout is never surfaced — the answer is
	// in the exit status — but a launchctl that never ran is not an answer:
	// only an error returns one, and a false with no error means launchctl
	// ran and did not find the job. A call cut short by ctx is an error too,
	// however far it got: an implementation that stops early knows nothing
	// about the job.
	list(ctx context.Context, label string) (bool, error)
	// printJob runs `launchctl print` against target (e.g.
	// "gui/501/com.y3owk1n.mimi") and returns what it said. Alone among these
	// calls, its stdout is the reason for making it: that text is where a
	// loaded job's pid and last exit status live.
	printJob(ctx context.Context, target string) (string, error)
	// bootstrap loads the plist at plistPath into domain (e.g. "gui/501").
	bootstrap(ctx context.Context, domain, plistPath string) error
	// bootout unloads target (e.g. "gui/501/com.y3owk1n.mimi").
	bootout(ctx context.Context, target string) error
	// start starts the already-loaded service named label.
	start(ctx context.Context, label string) error
	// stop stops the already-loaded service named label.
	stop(ctx context.Context, label string) error
}

// execLauncher is the launcher backed by the real launchctl binary on PATH.
type execLauncher struct{}

// list runs `launchctl list label`, whose exit status is the answer: zero for
// a job launchd holds, non-zero for one it does not.
//
// Only a non-zero exit says that, though. Everything else Run reports — a
// launchctl that is not on PATH, one that could not be spawned — is the
// command failing to ask rather than launchd answering no, and the two have to
// leave here as different things. *exec.ExitError is what tells them apart: it
// is returned only once the process has run and exited.
//
// The line is drawn there, and not at the exit status, on purpose. launchctl
// answers an absent job with 113 and an unreachable domain with 112, and
// neither number is documented or promised; reading a job as present because
// this one did not recognize its exit code would refuse installs on a machine
// with nothing wrong with it. So a launchctl that ran and failed for a reason
// of its own still reads as "job absent" here — the same answer it gave
// before, for the one case a process exit cannot separate.
//
// The context is the one thing that has to be consulted before any of that.
// CommandContext kills the process on cancellation, and a killed process
// exits like any other: without asking the context first, every canceled call
// would read as launchd saying the job is not there, which is the whole of what
// this classification exists to prevent.
func (execLauncher) list(ctx context.Context, label string) (bool, error) {
	err := exec.CommandContext(ctx, "launchctl", "list", label).Run()
	if err == nil {
		return true, nil
	}

	ctxErr := ctx.Err()
	if ctxErr != nil {
		return false, ctxErr
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return false, nil
	}

	return false, err
}

func (execLauncher) printJob(ctx context.Context, target string) (string, error) {
	out, err := exec.CommandContext(ctx, "launchctl", "print", target).Output()

	return string(out), err
}

func (execLauncher) bootstrap(ctx context.Context, domain, plistPath string) error {
	return exec.CommandContext(ctx, "launchctl", "bootstrap", domain, plistPath).Run()
}

func (execLauncher) bootout(ctx context.Context, target string) error {
	return exec.CommandContext(ctx, "launchctl", "bootout", target).Run()
}

func (execLauncher) start(ctx context.Context, label string) error {
	return exec.CommandContext(ctx, "launchctl", "start", label).Run()
}

func (execLauncher) stop(ctx context.Context, label string) error {
	return exec.CommandContext(ctx, "launchctl", "stop", label).Run()
}
