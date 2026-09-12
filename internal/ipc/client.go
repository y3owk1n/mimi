package ipc

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"time"

	"github.com/y3owk1n/mimi/internal/action"
	"github.com/y3owk1n/mimi/internal/config"
	derrors "github.com/y3owk1n/mimi/internal/errors"
	"github.com/y3owk1n/mimi/internal/paths"
)

const dialTimeout = 100 * time.Millisecond

// TryExecute sends a typed command to the daemon over the Unix socket when
// available. The command travels as it was built, so the daemon runs the very
// value the CLI validated. Returns CodeDaemonUnavailable when the daemon is
// not reachable so callers can fall back to direct execution against the same
// typed cmd.
func TryExecute(socketPath string, cmd action.Command) error {
	_, err := TryExecuteData(socketPath, cmd)

	return err
}

// TryExecuteData is TryExecute for the actions that answer with something,
// returning what the daemon read alongside the error. It is the same request
// on the same socket; only the reply is looked at further.
//
// There is no direct-path counterpart, and there cannot be: the actions that
// use this answer with state the daemon holds, so a CLI with no daemon to ask
// has nothing to report.
func TryExecuteData(socketPath string, cmd action.Command) (json.RawMessage, error) {
	socketPath = paths.ExpandHome(socketPath)

	_, err := os.Stat(socketPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, derrors.New(derrors.CodeDaemonUnavailable, "daemon socket not found")
		}

		return nil, derrors.Wrapf(err, derrors.CodeIPCFailed, "checking daemon socket")
	}

	dialer := net.Dialer{Timeout: dialTimeout}

	conn, err := dialer.DialContext(context.Background(), "unix", socketPath)
	if err != nil {
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			return nil, derrors.New(derrors.CodeDaemonUnavailable, "daemon socket timed out")
		}

		return nil, derrors.New(derrors.CodeDaemonUnavailable, "daemon not reachable")
	}

	defer func() { _ = conn.Close() }()

	reader := bufio.NewReader(conn)

	err = writeRequest(conn, Request{Version: ProtocolVersion, Command: cmd})
	if err != nil {
		return nil, err
	}

	resp, err := readResponse(reader)
	if err != nil {
		return nil, err
	}

	err = errorFromResponse(resp)
	if err != nil {
		return nil, err
	}

	return resp.Data, nil
}

// ResolveSocketPath returns the configured socket path when --config is set,
// otherwise the default socket path without reading config from disk.
func ResolveSocketPath(cliConfigPath string) string {
	if cliConfigPath == "" {
		return config.DefaultSocketPath
	}

	resolved := config.ResolvePath(cliConfigPath)

	cfg, err := config.Load(resolved)
	if err != nil {
		return config.DefaultSocketPath
	}

	return cfg.Settings.SocketFile
}
