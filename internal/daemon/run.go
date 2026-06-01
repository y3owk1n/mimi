//go:build darwin

package daemon

import (
	"context"
	"os"
	"syscall"

	"go.uber.org/zap"

	"github.com/y3owk1n/mimi/internal/config"
	"github.com/y3owk1n/mimi/internal/systray"
)

func runWithHost(
	cfg *config.Config,
	logger *zap.SugaredLogger,
	configPath string,
	version string,
) error {
	if !cfg.Systray.Enabled {
		return runCore(cfg, logger, configPath, nil)
	}

	quitCh := make(chan struct{})
	runDone := make(chan error, 1)

	requestQuit := func() {
		select {
		case <-quitCh:
		default:
			close(quitCh)
		}
	}

	reload := func(ctx context.Context, path string) error {
		process, err := os.FindProcess(os.Getpid())
		if err != nil {
			return err
		}

		return process.Signal(syscall.SIGHUP)
	}

	component := systray.NewComponent(version, configPath, reload, requestQuit, logger)

	go func() {
		err := runCore(cfg, logger, configPath, quitCh)

		systray.Quit()

		runDone <- err
	}()

	systray.Run(component.OnReady, component.OnExit)
	component.Close()

	return <-runDone
}
