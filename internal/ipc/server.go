package ipc

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/y3owk1n/mimi/internal/action"
	derrors "github.com/y3owk1n/mimi/internal/errors"
	"github.com/y3owk1n/mimi/internal/paths"
	"github.com/y3owk1n/mimi/internal/systray"
)

// actionEnqueueTimeout caps how long handleConn will block waiting for the
// action worker to accept a job. Without this, a stuck action worker (or a
// misbehaving client connection) can block indefinitely and prevent shutdown
// or starve other clients.
const actionEnqueueTimeout = 5 * time.Second

// Server accepts action requests over a Unix domain socket.
type Server struct {
	path string
	ln   net.Listener

	actionCh       chan actionJob
	enqueueTimeout time.Duration
	once           sync.Once
	shutdownOnce   sync.Once
}

type actionJob struct {
	name string
	args []string
	done chan error
}

// NewServer creates a Unix socket server at path.
func NewServer(path string) *Server {
	return &Server{
		path:           paths.ExpandHome(path),
		actionCh:       make(chan actionJob),
		enqueueTimeout: actionEnqueueTimeout,
	}
}

// Run listens for connections until ctx is canceled.
func (s *Server) Run(ctx context.Context) error {
	s.once.Do(s.startActionWorker)

	err := os.MkdirAll(filepath.Dir(s.path), 0o755) //nolint:mnd
	if err != nil {
		return derrors.Wrapf(err, derrors.CodeIPCFailed, "creating socket directory")
	}

	_ = os.Remove(s.path)

	lc := net.ListenConfig{}

	listener, err := lc.Listen(ctx, "unix", s.path)
	if err != nil {
		return derrors.Wrapf(err, derrors.CodeIPCFailed, "listening on socket")
	}

	s.ln = listener

	go func() {
		<-ctx.Done()

		_ = listener.Close()
		_ = os.Remove(s.path)
	}()

	for {
		conn, acceptErr := listener.Accept()
		if acceptErr != nil {
			select {
			case <-ctx.Done():
				return nil
			default:
				if errors.Is(acceptErr, net.ErrClosed) {
					return nil
				}

				continue
			}
		}

		go s.handleConn(conn)
	}
}

// Shutdown closes the action channel so the worker goroutine started by Run
// exits cleanly. Safe to call multiple times. Should be called after Run
// returns (typically via defer) to release the worker.
func (s *Server) Shutdown() {
	s.shutdownOnce.Do(func() {
		close(s.actionCh)
	})
}

func (s *Server) startActionWorker() {
	go func() {
		runtime.LockOSThread()

		for job := range s.actionCh {
			func() {
				defer func() {
					if r := recover(); r != nil {
						job.done <- fmt.Errorf("action worker panic: %v", r) //nolint:err113 // panic value is runtime-only
					}
				}()

				job.done <- action.Execute(job.name, job.args)
			}()
		}
	}()
}

func (s *Server) handleConn(conn net.Conn) {
	defer func() { _ = conn.Close() }()

	reader := bufio.NewReader(conn)

	req, err := readRequest(reader)
	if err != nil {
		_ = writeResponse(conn, responseFromError(err))

		return
	}

	done := make(chan error, 1)
	select {
	case s.actionCh <- actionJob{name: req.Action, args: req.Args, done: done}:
	case <-time.After(s.enqueueTimeout):
		_ = writeResponse(conn, responseFromError(derrors.New(
			derrors.CodeIPCFailed,
			"timed out enqueueing action",
		)))

		return
	}

	err = <-done
	if err == nil &&
		(req.Action == string(action.NameSpace) || req.Action == string(action.NameMoveWindowToSpace)) {
		systray.RefreshWorkspaceTitle()
	}

	_ = writeResponse(conn, responseFromError(err))
}
