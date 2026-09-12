package ipc

import (
	"bufio"
	"context"
	"encoding/json"
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
	// execute is the per-job action runner; defaults to action.ExecuteCommand
	// and is exposed as a field so tests can inject panicking/failing runners
	// to exercise the worker recovery and shutdown paths.
	execute func(cmd action.Command) error
	// direct holds the actions handled on the connection's own goroutine
	// instead of the action worker: the ones whose handler drives the desktop
	// through Serialize itself, and would deadlock waiting for the worker it
	// was running on.
	direct       map[action.Name]func(cmd action.Command) (json.RawMessage, error)
	directMu     sync.RWMutex
	once         sync.Once
	shutdownOnce sync.Once
}

type actionJob struct {
	run  func() error
	done chan error
}

// NewServer creates a Unix socket server at path.
func NewServer(path string) *Server {
	return &Server{
		path:           paths.ExpandHome(path),
		actionCh:       make(chan actionJob),
		enqueueTimeout: actionEnqueueTimeout,
		execute:        action.ExecuteCommand,
		direct:         map[action.Name]func(cmd action.Command) (json.RawMessage, error){},
	}
}

// HandleDirect routes every request for name to fn, run on the connection's
// goroutine rather than the action worker. fn may call Serialize.
func (s *Server) HandleDirect(
	name action.Name,
	fn func(cmd action.Command) (json.RawMessage, error),
) {
	s.directMu.Lock()
	defer s.directMu.Unlock()

	s.direct[name] = fn
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

// Serialize runs fn on the action worker, the one OS thread every action
// arriving over the socket runs on, and returns what it returned. It is how
// the daemon's own desktop work, the tiling engine's passes, stays in line
// with the actions the CLI sends: two callers driving the desktop at once
// would release each other's window references mid-action. It reports an
// error, without running fn, when the worker does not pick the job up in
// time.
func (s *Server) Serialize(fn func() error) error {
	s.once.Do(s.startActionWorker)

	done := make(chan error, 1)
	select {
	case s.actionCh <- actionJob{run: fn, done: done}:
	case <-time.After(s.enqueueTimeout):
		return derrors.New(derrors.CodeIPCFailed, "timed out enqueueing desktop work")
	}

	return <-done
}

// directHandler is the direct handler for name, if one is registered.
func (s *Server) directHandler(
	name action.Name,
) (func(cmd action.Command) (json.RawMessage, error), bool) {
	s.directMu.RLock()
	defer s.directMu.RUnlock()

	fn, ok := s.direct[name]

	return fn, ok
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

				job.done <- job.run()
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

	if handle, ok := s.directHandler(req.Command.Name); ok {
		data, handleErr := handle(req.Command)

		resp := responseFromError(handleErr)
		if handleErr == nil {
			resp.Data = data
		}

		_ = writeResponse(conn, resp)

		return
	}

	done := make(chan error, 1)
	select {
	case s.actionCh <- actionJob{run: func() error { return s.execute(req.Command) }, done: done}:
	case <-time.After(s.enqueueTimeout):
		_ = writeResponse(conn, responseFromError(derrors.New(
			derrors.CodeIPCFailed,
			"timed out enqueueing action",
		)))

		return
	}

	err = <-done

	_ = writeResponse(conn, responseFromError(err))
}
