package cmd

import (
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

	return err
}
