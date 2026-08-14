package cmd

import (
	"github.com/y3owk1n/mimi/internal/action"
	derrors "github.com/y3owk1n/mimi/internal/errors"
	"github.com/y3owk1n/mimi/internal/ipc"
)

// runAction sends an action to the daemon, falling back to executing it
// directly when no daemon is listening.
func (s *cliState) runAction(name string, args []string) error {
	socketPath := ipc.ResolveSocketPath(s.configPath)

	err := ipc.TryExecute(socketPath, name, args)
	if err == nil {
		return nil
	}

	if derrors.IsCode(err, derrors.CodeDaemonUnavailable) {
		return action.Execute(name, args)
	}

	return err
}
