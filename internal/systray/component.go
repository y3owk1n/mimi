package systray

import (
	"context"
	"os/exec"

	"go.uber.org/zap"
)

// Component owns Mimi's system tray menu.
type Component struct {
	version    string
	configPath string
	reload     func(context.Context, string) error
	quit       func()
	logger     *zap.SugaredLogger

	ctx    context.Context //nolint:containedctx // Ties menu event goroutine to tray lifecycle.
	cancel context.CancelFunc

	mVersion      *MenuItem
	mHelp         *MenuItem
	mSourceCode   *MenuItem
	mConfigDocs   *MenuItem
	mCLI          *MenuItem
	mReloadConfig *MenuItem
	mQuit         *MenuItem
}

// NewComponent creates a system tray menu component.
func NewComponent(
	version string,
	configPath string,
	reload func(context.Context, string) error,
	quit func(),
	logger *zap.SugaredLogger,
) *Component {
	ctx, cancel := context.WithCancel(context.Background())

	return &Component{
		version:    version,
		configPath: configPath,
		reload:     reload,
		quit:       quit,
		logger:     logger,
		ctx:        ctx,
		cancel:     cancel,
	}
}

// OnReady builds the tray menu when Cocoa is ready.
func (c *Component) OnReady() {
	c.mVersion = AddMenuItem("Version: " + c.version)

	c.mHelp = AddMenuItem("Help")
	c.mConfigDocs = c.mHelp.AddSubMenuItem("Config Docs")
	c.mCLI = c.mHelp.AddSubMenuItem("CLI Docs")
	c.mHelp.AddSeparator()
	c.mSourceCode = c.mHelp.AddSubMenuItem("Source Code")

	AddSeparator()

	c.mReloadConfig = AddMenuItem("Reload Config")

	AddSeparator()

	c.mQuit = AddMenuItem("Quit Mimi")

	SetTitle("M")
	SetTooltip("Mimi")

	go c.handleEvents()
}

// OnExit stops menu event handling.
func (c *Component) OnExit() {
	c.cancel()
}

// Close stops menu event handling.
func (c *Component) Close() {
	c.cancel()
}

func (c *Component) handleEvents() {
	for {
		select {
		case <-c.ctx.Done():
			return
		case <-c.mVersion.ClickedCh:
			c.logger.Infow("mimi version selected from systray", "version", c.version)
		case <-c.mSourceCode.ClickedCh:
			c.openURL("https://github.com/y3owk1n/mimi", "source code")
		case <-c.mConfigDocs.ClickedCh:
			c.openURL(
				"https://github.com/y3owk1n/mimi/blob/main/docs/CONFIGURATION.md",
				"configuration docs",
			)
		case <-c.mCLI.ClickedCh:
			c.openURL("https://github.com/y3owk1n/mimi/blob/main/docs/CLI.md", "CLI docs")
		case <-c.mReloadConfig.ClickedCh:
			c.handleReloadConfig()
		case <-c.mQuit.ClickedCh:
			c.quit()

			return
		}
	}
}

func (c *Component) openURL(url string, label string) {
	go func() {
		err := exec.CommandContext(c.ctx, "/usr/bin/open", url).Run()
		if err != nil {
			c.logger.Warnw("failed to open systray link", "label", label, "err", err)
		}
	}()
}

func (c *Component) handleReloadConfig() {
	err := c.reload(c.ctx, c.configPath)
	if err != nil {
		c.logger.Warnw("failed to reload config from systray", "err", err)

		return
	}

	c.logger.Info("configuration reloaded from systray")
}
