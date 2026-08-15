package service

import (
	"context"
	"os"
	"os/user"
	"path/filepath"

	derrors "github.com/y3owk1n/mimi/internal/errors"
	"github.com/y3owk1n/mimi/internal/paths"
)

const (
	dirPerm  = 0o755
	filePerm = 0o644
)

// Status is what checking the launchd service reports: a typed result a
// caller can act on, in place of a formatted string only a human can read.
type Status struct {
	// Loaded reports whether launchctl currently has the service loaded.
	Loaded bool
}

// Service manages the mimi launchd service. Every launchctl invocation runs
// through the launcher it holds.
type Service struct {
	launcher launcher
}

// New returns a Service backed by the real launchctl binary on PATH.
func New() *Service {
	return &Service{launcher: execLauncher{}}
}

// Install renders the plist for the running binary and configPath, writes it
// to ~/Library/LaunchAgents, and loads it with launchctl bootstrap.
func (s *Service) Install(configPath string) error {
	ctx := context.Background()

	if s.launcher.list(ctx, Label) == nil {
		return derrors.New(
			derrors.CodeServiceFailed,
			"service is already loaded; check for existing installations (e.g., nix-darwin, home-manager) and uninstall them first",
		)
	}

	binPath, err := binaryPath()
	if err != nil {
		return derrors.Wrapf(err, derrors.CodeServiceFailed, "getting binary path")
	}

	plistContent := renderPlist(binPath, configPath)

	expandedDir := paths.ExpandHome(launchAgentsDir)

	err = os.MkdirAll(expandedDir, dirPerm)
	if err != nil {
		return derrors.Wrapf(err, derrors.CodeServiceFailed, "creating LaunchAgents directory")
	}

	expandedPlist := filepath.Join(expandedDir, Label+".plist")

	err = writePlist(expandedPlist, plistContent)
	if err != nil {
		return err
	}

	currentUser, err := user.Current()
	if err != nil {
		return derrors.Wrapf(err, derrors.CodeServiceFailed, "getting current user")
	}

	err = s.launcher.bootstrap(ctx, "gui/"+currentUser.Uid, expandedPlist)
	if err != nil {
		return derrors.Wrapf(err, derrors.CodeServiceFailed, "loading service")
	}

	return nil
}

// writePlist creates path with content, using O_EXCL to create it atomically
// and avoid a TOCTOU race between checking for and writing the file. A
// partial write is rolled back rather than left behind.
func writePlist(path, content string) error {
	plistFile, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, filePerm)
	if err != nil {
		if os.IsExist(err) {
			return derrors.Newf(
				derrors.CodeServiceFailed,
				"plist file already exists at %s; remove it manually or uninstall first",
				path,
			)
		}

		return derrors.Wrapf(err, derrors.CodeServiceFailed, "creating plist")
	}

	_, err = plistFile.WriteString(content)
	if err != nil {
		_ = plistFile.Close()
		_ = os.Remove(path)

		return derrors.Wrapf(err, derrors.CodeServiceFailed, "writing plist")
	}

	err = plistFile.Close()
	if err != nil {
		return derrors.Wrapf(err, derrors.CodeServiceFailed, "closing plist")
	}

	return nil
}

// Uninstall unloads the service and removes its plist. A launchctl bootout
// failure is not fatal — the service may already be unloaded, e.g. after a
// prior partial uninstall — so it only removes the plist file.
func (s *Service) Uninstall() error {
	ctx := context.Background()

	expandedPlist := filepath.Join(paths.ExpandHome(launchAgentsDir), Label+".plist")

	currentUser, err := user.Current()
	if err != nil {
		return derrors.Wrapf(err, derrors.CodeServiceFailed, "getting current user")
	}

	_ = s.launcher.bootout(ctx, "gui/"+currentUser.Uid+"/"+Label)

	err = os.Remove(expandedPlist)
	if err != nil && !os.IsNotExist(err) {
		return derrors.Wrapf(err, derrors.CodeServiceFailed, "removing plist")
	}

	return nil
}

// Start starts the already-installed service.
func (s *Service) Start() error {
	err := s.launcher.start(context.Background(), Label)
	if err != nil {
		return derrors.Wrapf(err, derrors.CodeServiceFailed, "starting service")
	}

	return nil
}

// Stop stops the running service.
func (s *Service) Stop() error {
	err := s.launcher.stop(context.Background(), Label)
	if err != nil {
		return derrors.Wrapf(err, derrors.CodeServiceFailed, "stopping service")
	}

	return nil
}

// Restart stops then starts the service. A failure to stop (e.g. it was not
// running) does not prevent the start that follows.
func (s *Service) Restart() error {
	_ = s.Stop()

	return s.Start()
}

// Status reports whether the service is currently loaded.
func (s *Service) Status() Status {
	return Status{Loaded: s.launcher.list(context.Background(), Label) == nil}
}

// binaryPath resolves the real, symlink-free path to the running mimi
// binary, which is what the installed plist must invoke.
func binaryPath() (string, error) {
	execPath, err := os.Executable()
	if err != nil {
		return "", err
	}

	return filepath.EvalSymlinks(execPath)
}
