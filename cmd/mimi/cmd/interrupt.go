package cmd

import (
	"context"
	"os"

	"github.com/spf13/cobra"
)

// interruptBufferSize is how many interrupts the watch can hold at once. Two,
// because the second is the one that ends the process and signal delivery drops
// what a full channel cannot take: an interrupt that lands while the first is
// still being taken has to have somewhere to sit, or a user pressing Ctrl-C
// twice in quick succession would be pressing it once.
const interruptBufferSize = 2

// interruptExitCode is what mimi exits with when a second interrupt ends it:
// the 128 + SIGINT a shell reports for a process the signal killed, which is
// what every mimi command reported on the first Ctrl-C before this watch
// existed.
const interruptExitCode = 130

// exitProcess ends the process the way the second interrupt asks it to, with no
// deferred cleanup and nothing left to finish. It is the forceExit [Execute]
// hands the watch, and is a function of its own only so that a test can be
// handed something that returns.
func exitProcess() {
	os.Exit(interruptExitCode)
}

// runUnderInterrupts runs root under a context that the first interrupt on
// signals cancels, and hands the second to forceExit.
//
// The context reaches the command through cobra, which passes the one given to
// ExecuteContext down to whatever it runs. Nothing about when cobra parses
// flags, validates arguments or runs a persistent pre-run moves: ExecuteContext
// only records the context before handing over to the same Execute the tree
// used to run under, so the boundary mimi#175 put between a bad command line
// and a runtime failure stays exactly where it was.
func runUnderInterrupts(
	root *cobra.Command,
	signals <-chan os.Signal,
	forceExit func(),
) error {
	ctx, release := interruptContext(context.Background(), signals, forceExit)
	defer release()

	return root.ExecuteContext(ctx)
}

// interruptContext derives a context from parent that the first signal on
// signals cancels, arms the second to run forceExit, and returns the function
// that releases the watch.
//
// The two stages exist because watching a signal takes Go's own response to it
// away. Every mimi command used to die on the first Ctrl-C, whatever it was
// doing, because nothing in the process was watching SIGINT and the runtime
// terminated it; a watch that only canceled would leave every command that
// does not read its context — most of `mimi action` and `mimi config`, per the
// audit in interrupt_audit_test.go — running with no way left to stop it. So
// the first interrupt asks the command to stop and the second stops the
// process, and the guarantee the CLI gives is unchanged in the only terms a
// user cares about: Ctrl-C ends the command.
//
// The release function cancels the context and returns only once the watch has
// stopped, so nothing it drives can fire after it has returned. An interrupt
// that lands as the release does can still win the race and end the process,
// which is the outcome that interrupt asks for either way.
func interruptContext(
	parent context.Context,
	signals <-chan os.Signal,
	forceExit func(),
) (context.Context, func()) {
	ctx, cancel := context.WithCancel(parent)

	released := make(chan struct{})
	stopped := make(chan struct{})

	go func() {
		defer close(stopped)

		select {
		case <-signals:
			cancel()
		case <-released:
			return
		}

		select {
		case <-signals:
			forceExit()
		case <-released:
		}
	}()

	return ctx, func() {
		close(released)
		<-stopped
		cancel()
	}
}
