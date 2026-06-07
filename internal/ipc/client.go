package ipc

import (
	"bufio"
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/y3owk1n/mimi/internal/config"
	derrors "github.com/y3owk1n/mimi/internal/errors"
)

const dialTimeout = 100 * time.Millisecond

// TryExecute sends an action to the daemon over the Unix socket when available.
// Returns CodeDaemonUnavailable when the daemon is not reachable so callers can
// fall back to direct execution.
func TryExecute(socketPath, action string, args []string) error {
	socketPath = expandHome(socketPath)

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

	err = writeRequest(conn, Request{Action: action, Args: args})
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

func expandHome(path string) string {
	if strings.HasPrefix(path, "~") {
		home, _ := os.UserHomeDir()

		return filepath.Join(home, path[1:])
	}

	return path
}
