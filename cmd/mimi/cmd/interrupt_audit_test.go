//nolint:testpackage
package cmd

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// interruptResponse is what the first Ctrl-C does to one command.
//
// It is a string rather than a counter because the value is read by whoever is
// reading the audit, and a number would send them back here to find out which
// of three things it meant.
type interruptResponse string

const (
	// interruptStopsTheWork is a command that reads the context it was given.
	// The interrupt reaches the work in progress and ends it, and the command
	// reports why it stopped.
	interruptStopsTheWork interruptResponse = "stops the work in progress"
	// interruptShutsDownTheDaemon is `mimi start`, which has watched SIGINT
	// itself since long before the CLI did. The root's watch does not take
	// that away — signal delivery goes to every channel registered for it — so
	// the daemon still shuts down gracefully on the first Ctrl-C.
	interruptShutsDownTheDaemon interruptResponse = "shuts the daemon down through its own signal handler"
	// interruptRunsOn is a command that does not read its context. The first
	// interrupt does not reach it and it finishes what it started; the second
	// ends the process. Every entry recorded this way carries what the command
	// is doing instead, which is what a reader needs to judge whether the
	// second Ctrl-C is an acceptable answer for it.
	interruptRunsOn interruptResponse = "runs on; the second interrupt ends the process"
)

// auditEntry is one command's line in the audit.
type auditEntry struct {
	// command is the command as it is typed, without the leading "mimi".
	command string
	// response is what the first interrupt does.
	response interruptResponse
	// why is the finding: what the command actually does with the interrupt,
	// and — for interruptRunsOn — why running on is acceptable. It is what
	// makes this an audit rather than a list, and it is checked by a person,
	// not by the test below.
	why string
}

// audited is one line of the audit, written as a call so that each line reads
// as the sentence it is.
func audited(command string, response interruptResponse, why string) auditEntry {
	return auditEntry{command: command, response: response, why: why}
}

// interruptAudit is the audit the ticket (mimi#173) asks be recorded rather
// than assumed: every runnable command in the tree, read for what it does with
// the context [Execute] now hands it.
//
// It is recorded here, next to a test that fails when the tree and the audit
// disagree, because a command added later inherits this wiring whether or not
// anyone thinks about it — and the failure mode it inherits, an interrupt that
// no longer reaches the work, is invisible from the command's own code.
//
// What the audit found: six commands honor the context, one runs its own
// signal handler, and eleven ignore it. Nine of those eleven are local file
// work or one-shot syscalls, over before an interrupt could be typed. The two
// that are not are in the action family — `action space` on the direct path
// pumps the run loop for a stretch proportional to the spaces it crosses, and
// any action on the daemon path waits for a reply that ipc.TryExecute reads
// with no deadline, so a daemon that accepts the connection and then goes quiet
// hangs it indefinitely. Neither is a wait a Go context could shorten from
// here: the first is inside Objective-C, and the second wants a read deadline
// in internal/ipc rather than a cancellation in the CLI. Both are why the
// second stage exists, and neither is made worse by this change — before it,
// one Ctrl-C ended them; after it, two do.
//
//nolint:gochecknoglobals // the audit is one fact about the tree, read by one test
var interruptAudit = []auditEntry{
	audited("action", interruptRunsOn,
		"the body only reports the missing subcommand and returns; "+
			"there is no window in which an interrupt could arrive"),
	audited("action focus_window", interruptRunsOn,
		"runAction consults no context on either path: the daemon path "+
			"writes a request and waits for the reply, the direct path calls "+
			"into Objective-C that runs to completion"),
	audited("action space", interruptRunsOn,
		"as focus_window, and the direct path's synthetic dock swipe "+
			"pumps the run loop for a stretch proportional to the spaces it "+
			"crosses — time inside C that a canceled Go context could not "+
			"shorten even if runAction read one"),
	audited("action move_window_to_space", interruptRunsOn,
		"as space; the SkyLight move likewise runs to completion"),
	audited("action resize_window", interruptRunsOn,
		"as focus_window; the Accessibility resize runs to completion"),
	audited("config dump", interruptRunsOn,
		"reads and parses one local file and prints it"),
	audited("config init", interruptRunsOn,
		"writes one local file"),
	audited("config reload", interruptRunsOn,
		"reads the config and the PID file and sends one SIGHUP; "+
			"nothing here waits for the daemon to finish reloading"),
	audited("config validate", interruptRunsOn,
		"reads and parses one local file and reports on it"),
	audited("services install", interruptStopsTheWork,
		"Service.Install threads the context through every launchctl call "+
			"and checks it again before the plist is written, so a canceled "+
			"install stops on whichever side of that write it had reached"),
	audited("services uninstall", interruptStopsTheWork,
		"Service.Uninstall checks the context after the bootout and keeps "+
			"the plist, so the uninstall can be run again"),
	audited("services start", interruptStopsTheWork,
		"the launchctl call runs under the context and is killed with it"),
	audited("services stop", interruptStopsTheWork,
		"the launchctl call runs under the context and is killed with it"),
	audited("services restart", interruptStopsTheWork,
		"the launchctl calls run under the context and are killed with it"),
	audited("services status", interruptStopsTheWork,
		"the launchctl calls run under the context; a canceled status "+
			"stops asking and prints the unknown state rather than failing, "+
			"which is what it prints for every launchctl it cannot reach"),
	audited("start", interruptShutsDownTheDaemon,
		"daemon.Run watches SIGINT on a channel of its own and shuts down "+
			"on it. Before the daemon reaches that watch — while the config "+
			"onboarding alert is up, say — the first interrupt does nothing "+
			"and the second ends the process"),
	audited("status", interruptRunsOn,
		"reads the PID file, asks the Accessibility API whether mimi is "+
			"trusted, and stats the socket; none of the three blocks"),
	audited("stop", interruptRunsOn,
		"reads the PID file and sends one SIGTERM"),
}

// TestNewRootCmd_EveryCommandsInterruptResponseIsAudited is the ticket's second
// acceptance criterion, kept true rather than written down once: the tree and
// the audit have to name the same commands.
//
// A command missing from the audit is the case this exists for. It has been
// given a context by [Execute] and taken Go's default response to Ctrl-C away
// in exchange, and nobody has said what it does with either.
func TestNewRootCmd_EveryCommandsInterruptResponseIsAudited(t *testing.T) {
	paths := runnableCommandPaths(newRootCmd())
	if len(paths) == 0 {
		t.Fatal("found no runnable commands in the tree")
	}

	findings := make(map[string]auditEntry, len(interruptAudit))

	for _, entry := range interruptAudit {
		_, duplicate := findings[entry.command]
		if duplicate {
			t.Errorf("the interrupt audit records %q twice", entry.command)
		}

		findings[entry.command] = entry
	}

	inTree := make(map[string]bool, len(paths))

	for _, path := range paths {
		name := strings.Join(path, " ")
		inTree[name] = true

		entry, ok := findings[name]
		if !ok {
			t.Errorf(
				"%q is not in the interrupt audit; record what the first Ctrl-C does to it",
				name,
			)

			continue
		}

		if entry.why == "" {
			t.Errorf("%q is in the interrupt audit with no finding recorded", name)
		}

		switch entry.response {
		case interruptStopsTheWork, interruptShutsDownTheDaemon, interruptRunsOn:
		default:
			t.Errorf("%q records the unknown interrupt response %q", name, entry.response)
		}
	}

	// A stale entry is worth failing on too: an audit that outlives the command
	// it describes is one nobody has re-read.
	var stale []string

	for name := range findings {
		if !inTree[name] {
			stale = append(stale, name)
		}
	}

	sort.Strings(stale)

	for _, name := range stale {
		t.Errorf("the interrupt audit records %q, which is not a command in the tree", name)
	}
}

// TestNewRootCmd_EveryCommandIsGivenTheCallersContext is the audit's
// precondition. Every finding above is about what a command does with the
// context it was handed, which is worth nothing unless every command is in fact
// handed it — including the leaves under `action`, `config` and `services`,
// whose parents have no body of their own for cobra to reach them through.
func TestNewRootCmd_EveryCommandIsGivenTheCallersContext(t *testing.T) {
	xdg := isolateConfigHome(t)
	writeConfig(t, filepath.Join(xdg, "mimi", "config.toml"), "/marker/default.log")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	paths := runnableCommandPaths(newRootCmd())
	if len(paths) == 0 {
		t.Fatal("found no runnable commands in the tree")
	}

	for _, path := range paths {
		t.Run(strings.Join(path, " "), func(t *testing.T) {
			var seen error

			err := runStubbedWithContext(t, ctx, path, func(cobraCmd *cobra.Command) error {
				seen = cobraCmd.Context().Err()

				return nil
			})
			if err != nil {
				t.Fatalf("running command: %v", err)
			}

			if seen == nil {
				t.Error("the command ran on a context of its own, not the caller's")
			}
		})
	}
}

// TestServicesCommands_UnderACanceledContextChangeNothing is the ticket's
// fourth acceptance criterion where the CLI can reach it: an interrupt that
// arrives before a services command has done anything leaves the disk as it
// found it, and reports a failure rather than a success.
//
// What it does not cover is the interrupt that arrives partway through an
// install, between the unload and the write — that one belongs to the service
// itself and is covered there, by
// TestService_Install_WritesNoPlistWhenCanceledAfterTheUnload.
//
// What it adds over those is the real launcher. Nothing here fakes launchctl
// and nothing here runs it either: exec refuses to start a process under a
// context that is already done, which is exactly the shape the classification
// in execLauncher.list has to survive. A killed launchctl exits like a
// launchctl that looked and found no job, and reading the one as the other is
// what would let an interrupted install bootstrap over a service that is still
// up. Until this wiring existed, only a synthetic context ever put that guard
// under a real *exec.ExitError.
func TestServicesCommands_UnderACanceledContextChangeNothing(t *testing.T) {
	// Written as one string so that the subcommand names are not five more
	// bare literals in this package for goconst to count.
	names := strings.FieldsSeq("install uninstall start stop restart")

	for name := range names {
		t.Run(name, func(t *testing.T) {
			isolateConfigHome(t)

			plist := filepath.Join(
				os.Getenv("HOME"),
				"Library",
				"LaunchAgents",
				"com.y3owk1n.mimi.plist",
			)

			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			err := runWithContext(t, ctx, "services", name)
			if err == nil {
				t.Fatalf("mimi services %s reported success under a canceled context", name)
			}

			_, statErr := os.Stat(plist)
			if statErr == nil {
				t.Errorf("mimi services %s wrote a plist under a canceled context", name)
			}
		})
	}
}
