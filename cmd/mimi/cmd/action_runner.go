package cmd

import (
	"github.com/y3owk1n/mimi/internal/action"
	derrors "github.com/y3owk1n/mimi/internal/errors"
	"github.com/y3owk1n/mimi/internal/ipc"
)

func runAction(name string, args []string) error {
	socketPath := ipc.ResolveSocketPath(configPath)

	err := ipc.TryExecute(socketPath, name, args)
	if err == nil {
		return nil
	}

	if derrors.IsCode(err, derrors.CodeDaemonUnavailable) {
		return action.Execute(name, args)
	}

	return err
}
