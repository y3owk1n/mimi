package cmd

import (
	"encoding/json"

	"github.com/y3owk1n/mimi/internal/action"
	"github.com/y3owk1n/mimi/internal/daemon"
	derrors "github.com/y3owk1n/mimi/internal/errors"
	"github.com/y3owk1n/mimi/internal/ipc"
)

// probeDaemon asks the daemon at socketPath what build it is. The error is
// CodeDaemonUnavailable when nothing answered, CodeProtocolMismatch when a
// daemon of another build refused the request, and CodeInvalidInput when a
// daemon built before the status request did not know it.
func probeDaemon(socketPath string) (daemon.Status, error) {
	data, err := ipc.TryExecuteData(socketPath, action.NewStatusCommand())
	if err != nil {
		return daemon.Status{}, err
	}

	var status daemon.Status

	err = json.Unmarshal(data, &status)
	if err != nil {
		return daemon.Status{}, derrors.Wrapf(
			err,
			derrors.CodeSerializationFailed,
			"decoding daemon status",
		)
	}

	return status, nil
}
