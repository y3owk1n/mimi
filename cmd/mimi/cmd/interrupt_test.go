//nolint:testpackage
package cmd

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// runWithContext runs args against a fresh command tree under ctx and reports
// how the run ended. It is runCommand with a context, for the tests about what
// a canceled one does to a command that runs for real.
func runWithContext(t *testing.T, ctx context.Context, args ...string) error {
	t.Helper()

	root := newRootCmd()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs(args)

	return root.ExecuteContext(ctx)
}

// runStubbedWithContext runs the command at path against a fresh tree under
// ctx, with that command's body replaced by stub. Unlike runWithStubbedBody the
// stub is handed the command cobra is running rather than the tree's root,
// because what these tests read is the context that command was given.
func runStubbedWithContext(
	t *testing.T,
	ctx context.Context,
	path []string,
	stub func(cobraCmd *cobra.Command) error,
) error {
	t.Helper()

	root := newRootCmd()

	target, _, err := root.Find(path)
	if err != nil {
		t.Fatalf("finding command: %v", err)
	}

	target.Args = cobra.ArbitraryArgs
	target.RunE = func(cobraCmd *cobra.Command, _ []string) error {
		return stub(cobraCmd)
	}

	root.SetArgs(path)
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)

	return root.ExecuteContext(ctx)
}

// TestInterruptContext_TheFirstInterruptCancelsTheContext is the whole point of
// the wiring: an interrupt has to reach the command as a canceled context, not
// only as a dead process.
func TestInterruptContext_TheFirstInterruptCancelsTheContext(t *testing.T) {
	signals := make(chan os.Signal, 1)

	ctx, release := interruptContext(context.Background(), signals, func() {
		t.Error("the first interrupt ended the process instead of canceling")
	})
	defer release()

	if ctx.Err() != nil {
		t.Fatalf("the context was canceled before any interrupt: %v", ctx.Err())
	}

	signals <- os.Interrupt

	<-ctx.Done()
}

// TestInterruptContext_TheSecondInterruptEndsTheProcess is the promise that
// keeps a command which ignores its context from becoming unkillable. Before
// this wiring every command died on the first Ctrl-C, because Go terminated the
// process; watching the signal takes that away, and the second one gives it
// back.
func TestInterruptContext_TheSecondInterruptEndsTheProcess(t *testing.T) {
	signals := make(chan os.Signal, 2)
	forced := make(chan struct{})

	ctx, release := interruptContext(context.Background(), signals, func() {
		close(forced)
	})
	defer release()

	signals <- os.Interrupt

	// The first interrupt has to have been taken before the second is sent, or
	// a watch that read both from the buffer at once would pass this without
	// ever having staged them.
	<-ctx.Done()

	signals <- os.Interrupt

	<-forced
}

// TestInterruptContext_ReleasingEndsTheWatchWithoutForcingAnExit covers the
// ordinary end of every mimi run: the command finished, and the interrupt watch
// has to go with it. A watch that outlived the release would turn the next
// Ctrl-C at the shell prompt into an exit from a process that is already done.
func TestInterruptContext_ReleasingEndsTheWatchWithoutForcingAnExit(t *testing.T) {
	testCases := []struct {
		name       string
		interrupts int
	}{
		{name: "with no interrupt at all", interrupts: 0},
		{name: "with one interrupt already taken", interrupts: 1},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			signals := make(chan os.Signal, 1)

			ctx, release := interruptContext(context.Background(), signals, func() {
				t.Error("releasing the watch ended the process")
			})

			for range testCase.interrupts {
				signals <- os.Interrupt

				<-ctx.Done()
			}

			// Release returns only once the watch has stopped, so anything it
			// could still have done has been done by the time this returns.
			release()

			if ctx.Err() == nil {
				t.Error("releasing the watch left the context uncanceled")
			}
		})
	}
}

// TestRunUnderInterrupts_AnInterruptCancelsTheRunningCommand is the ticket's
// first acceptance criterion end to end: the command mimi is running sees the
// user's interrupt as a canceled context, and the run ends because the command
// stopped rather than because the process was killed under it.
func TestRunUnderInterrupts_AnInterruptCancelsTheRunningCommand(t *testing.T) {
	isolateConfigHome(t)

	signals := make(chan os.Signal, 1)
	entered := make(chan struct{})

	// Any command in the tree would do; this one is picked for having no
	// effect of its own, since its body is replaced below either way.
	path := strings.Fields("config dump")

	root := newRootCmd()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs(path)

	target, _, err := root.Find(path)
	if err != nil {
		t.Fatalf("finding command: %v", err)
	}

	// Blocking on the context is what a command that honors it does; the real
	// one this stands in for is the unload wait inside `mimi services install`,
	// which cannot be driven from here without a launchctl underneath it.
	target.RunE = func(cobraCmd *cobra.Command, _ []string) error {
		close(entered)
		<-cobraCmd.Context().Done()

		return cobraCmd.Context().Err()
	}

	go func() {
		<-entered

		signals <- os.Interrupt
	}()

	err = runUnderInterrupts(root, signals, func() {
		t.Error("one interrupt ended the process instead of canceling the command")
	})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("the interrupted command returned %v, want a canceled context", err)
	}
}

// TestRunUnderInterrupts_ASecondInterruptEndsACommandThatIgnoresItsContext is
// the ticket's third acceptance criterion, for the eleven commands the audit
// records as not reading their context.
//
// Watching SIGINT takes Go's own response to it away, so for those commands the
// first interrupt is the one that now does nothing. This is what stops that
// from making them unkillable, driven through the same runUnderInterrupts the
// CLI runs every command under.
func TestRunUnderInterrupts_ASecondInterruptEndsACommandThatIgnoresItsContext(t *testing.T) {
	isolateConfigHome(t)

	signals := make(chan os.Signal, interruptBufferSize)
	entered := make(chan struct{})
	ended := make(chan struct{})

	path := strings.Fields("config dump")

	root := newRootCmd()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs(path)

	target, _, err := root.Find(path)
	if err != nil {
		t.Fatalf("finding command: %v", err)
	}

	// canceledWhenEnded is read by the assertion below, and written by a body
	// that stands in for a command which ignores its context: it consults the
	// context nowhere, and nothing short of the process ending stops it.
	var canceledWhenEnded bool

	target.RunE = func(cobraCmd *cobra.Command, _ []string) error {
		close(entered)
		<-ended

		canceledWhenEnded = cobraCmd.Context().Err() != nil

		return nil
	}

	go func() {
		<-entered

		signals <- os.Interrupt

		signals <- os.Interrupt
	}()

	// Standing in for os.Exit, which a test cannot be handed. Releasing the
	// command is what the real one does to it, only harder.
	err = runUnderInterrupts(root, signals, func() { close(ended) })
	if err != nil {
		t.Fatalf("the command reported %v", err)
	}

	// The command was still running when the second interrupt arrived, and the
	// first had already canceled the context it was ignoring. Both halves of
	// the criterion, in one fact.
	if !canceledWhenEnded {
		t.Error("the process was ended before the first interrupt had canceled anything")
	}
}
