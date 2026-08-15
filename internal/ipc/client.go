package ipc

import (
	"bufio"
	"context"
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
// available, marshaling it to the wire's string args itself. Returns
// CodeDaemonUnavailable when the daemon is not reachable so callers can fall
// back to direct execution against the same typed cmd.
func TryExecute(socketPath string, cmd action.Command) error {
	socketPath = paths.ExpandHome(socketPath)

	_, err := os.Stat(socketPath)
	if err != nil {
		if os.IsNotExist(err) {
			return derrors.New(derrors.CodeDaemonUnavailable, "daemon socket not found")
		}

		return derrors.Wrapf(err, derrors.CodeIPCFailed, "checking daemon socket")
	}

	dialer := net.Dialer{Timeout: dialTimeout}

	conn, err := dialer.DialContext(context.Background(), "unix", socketPath)
	if err != nil {
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			return derrors.New(derrors.CodeDaemonUnavailable, "daemon socket timed out")
		}

		return derrors.New(derrors.CodeDaemonUnavailable, "daemon not reachable")
	}

	defer func() { _ = conn.Close() }()

	reader := bufio.NewReader(conn)

	name, args := marshalCommand(cmd)

	err = writeRequest(conn, Request{Action: name, Args: args})
	if err != nil {
		return err
	}

	resp, err := readResponse(reader)
	if err != nil {
		return err
	}

	return errorFromResponse(resp)
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
