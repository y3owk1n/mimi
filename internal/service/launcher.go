package service

import (
	"context"
	"os/exec"
)

// launcher is the seam between [Service] and launchctl. The production
// implementation shells out; tests fake it so install/uninstall/start/stop/
// status can be exercised without a real launchctl underneath them.
type launcher interface {
	// list reports whether label is currently loaded. Every caller here only
	// cares whether it succeeded, so its stdout is never surfaced.
	list(ctx context.Context, label string) error
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

func (execLauncher) list(ctx context.Context, label string) error {
	return exec.CommandContext(ctx, "launchctl", "list", label).Run()
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
