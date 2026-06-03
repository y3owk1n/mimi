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
	var (
		quitCh    <-chan struct{}
		component *systray.Component
	)

	runDone := make(chan error, 1)

	reload := func(ctx context.Context, path string) error {
		process, err := os.FindProcess(os.Getpid())
		if err != nil {
			return err
		}

		return process.Signal(syscall.SIGHUP)
	}

	if cfg.Systray.Enabled {
		quitChWritable := make(chan struct{})
		quitCh = quitChWritable

		requestQuit := func() {
			select {
			case <-quitChWritable:
			default:
				close(quitChWritable)
			}
		}

		component = systray.NewComponent(
			version,
			configPath,
			reload,
			requestQuit,
			cfg.Systray.ShowWorkspaceNumber,
			logger,
		)
	}

	go func() {
		err := runCore(cfg, logger, configPath, quitCh)

		systray.Quit()

		runDone <- err
	}()

	if cfg.Systray.Enabled {
		systray.Run(component.OnReady, component.OnExit)
		component.Close()
	} else {
		systray.RunHeadless(func() {}, func() {})
	}

	return <-runDone
}
