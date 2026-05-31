package hooks

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/y3owk1n/mimi/internal/config"
	"github.com/y3owk1n/mimi/internal/events"
)

type Executor struct {
	registry *Registry
	cfg      *config.SettingsConfig
	logger   *slog.Logger
	sem      chan struct{}
	cfgMu    sync.RWMutex
}

func NewExecutor(reg *Registry, cfg *config.SettingsConfig, logger *slog.Logger) *Executor {
	return &Executor{
		registry: reg,
		cfg:      cfg,
		logger:   logger,
		sem:      make(chan struct{}, cfg.MaxHookWorkers),
	}
}

func (ex *Executor) UpdateSettings(cfg *config.SettingsConfig) {
	ex.cfgMu.Lock()
	ex.cfg = cfg
	ex.cfgMu.Unlock()
}

func (ex *Executor) Handle(e events.Event) {
	hooks := ex.registry.HooksFor(e.Kind)
	for _, h := range hooks {
		if !h.Matches(e) {
			continue
		}
		if h.Entry.Async {
			go ex.run(h, e)
		} else {
			ex.run(h, e)
		}
	}
}

func (ex *Executor) Run(ctx context.Context, sub events.Subscriber) {
	for {
		select {
		case <-ctx.Done():
			return
		case e, ok := <-sub:
			if !ok {
				return
			}
			ex.Handle(e)
		}
	}
}

func (ex *Executor) run(h Hook, e events.Event) {
	ex.sem <- struct{}{}
	defer func() { <-ex.sem }()

	ex.cfgMu.RLock()
	shell := ex.cfg.HookShell
	timeout := time.Duration(ex.cfg.HookTimeoutSecs) * time.Second
	ex.cfgMu.RUnlock()

	if h.Entry.TimeoutSecs > 0 {
		timeout = time.Duration(h.Entry.TimeoutSecs) * time.Second
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, shell, "-c", h.Entry.Run)
	cmd.Env = append(os.Environ(), eventEnv(e)...)

	start := time.Now()
	out, err := cmd.CombinedOutput()
	elapsed := time.Since(start)

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			ex.logger.Warn("hook timed out",
				"kind", e.Kind, "cmd", h.Entry.Run, "timeout", timeout)
		} else {
			ex.logger.Error("hook failed",
				"kind", e.Kind, "cmd", h.Entry.Run,
				"exit", err, "output", strings.TrimSpace(string(out)))
		}
	} else {
		output := strings.TrimSpace(string(out))
		attrs := []any{
			"kind", e.Kind,
			"cmd", h.Entry.Run,
			"elapsed", elapsed.Round(time.Millisecond),
		}
		if output != "" {
			attrs = append(attrs, "output", output)
		}
		ex.logger.Info("hook ok", attrs...)
	}
}

func eventEnv(e events.Event) []string {
	vars := []string{
		fmt.Sprintf("mimi_EVENT=%s", e.Kind),
		fmt.Sprintf("mimi_EVENT_ID=%s", e.ID),
		fmt.Sprintf("mimi_APP_NAME=%s", e.AppName),
		fmt.Sprintf("mimi_BUNDLE_ID=%s", e.BundleID),
		fmt.Sprintf("mimi_PID=%d", e.PID),
		fmt.Sprintf("mimi_WINDOW_TITLE=%s", e.WindowTitle),
		fmt.Sprintf("mimi_VOLUME_PATH=%s", e.VolumePath),
		fmt.Sprintf("mimi_VOLUME_NAME=%s", e.VolumeName),
		fmt.Sprintf("mimi_TIMESTAMP=%s", e.At.Format(time.RFC3339)),
	}
	for k, v := range e.Extra {
		vars = append(vars, fmt.Sprintf("mimi_%s=%s", strings.ToUpper(k), v))
	}
	return vars
}
