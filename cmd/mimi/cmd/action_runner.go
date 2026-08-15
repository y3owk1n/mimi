package cmd

import (
	"fmt"

	"github.com/y3owk1n/mimi/internal/action"
	derrors "github.com/y3owk1n/mimi/internal/errors"
	"github.com/y3owk1n/mimi/internal/ipc"
)

// runAction sends a typed command to the daemon, falling back to running it
// directly when no daemon is listening. The command is built once and both
// paths run that same value: the daemon path puts it on the socket as JSON,
// and the direct path runs it where it stands. Neither re-parses a string the
// other already held typed.
func (s *cliState) runAction(cmd action.Command) error {
	socketPath := ipc.ResolveSocketPath(s.configPath)

	err := ipc.TryExecute(socketPath, cmd)
	if err == nil {
		return nil
	}

	if derrors.IsCode(err, derrors.CodeDaemonUnavailable) {
		return action.ExecuteCommand(cmd)
	}

	// A daemon left running across an upgrade speaks a different request
	// envelope than this build, and says so under a code of its own. The
	// command still runs — on the direct path, exactly as it does when no
	// daemon is listening — because hard-erroring here would break every
	// hotkey until someone restarts the daemon, for a condition the direct
	// path handles fine. Falling back silently was rejected too: the skew
	// would then last until the next reboot with nothing ever mentioning it.
	if derrors.IsCode(err, derrors.CodeProtocolMismatch) {
		_, _ = fmt.Fprintf(
			s.warnWriter(),
			"mimi: the daemon speaks a different request protocol than this CLI (%s) — restart the daemon so it runs this build ('mimi stop && mimi start', or 'mimi services restart' when installed as a service). This action ran on the direct path instead.\n",
			derrors.Message(err),
		)

		return action.ExecuteCommand(cmd)
	}

	return err
}
