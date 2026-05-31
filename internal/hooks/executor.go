package hooks

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"regexp"
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

	runCmd := replaceEventVars(h.Entry.Run, eventEnvMap(e))
	cmd := exec.CommandContext(ctx, shell, "-c", runCmd)

	cmd.Env = append(os.Environ(), eventEnv(e)...)

	start := time.Now()
	out, err := cmd.CombinedOutput()
	elapsed := time.Since(start)

	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
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
		"mimi_EVENT_ID=" + e.ID,
		"mimi_APP_NAME=" + e.AppName,
		"mimi_BUNDLE_ID=" + e.BundleID,
		fmt.Sprintf("mimi_PID=%d", e.PID),
		"mimi_WINDOW_TITLE=" + e.WindowTitle,
		"mimi_VOLUME_PATH=" + e.VolumePath,
		"mimi_VOLUME_NAME=" + e.VolumeName,
		"mimi_TIMESTAMP=" + e.At.Format(time.RFC3339),
	}
	for k, v := range e.Extra {
		vars = append(vars, fmt.Sprintf("mimi_%s=%s", strings.ToUpper(k), v))
	}

	return vars
}

func eventEnvMap(e events.Event) map[string]string {
	env := eventEnv(e)

	m := make(map[string]string, len(env))
	for _, kv := range env {
		parts := strings.SplitN(kv, "=", 2)
		if len(parts) == 2 {
			m[parts[0]] = parts[1]
		}
	}

	return m
}

var mimiVarRegex = regexp.MustCompile(`\${mimi_[A-Za-z0-9_]+}|\$mimi_[A-Za-z0-9_]+`)

func replaceEventVars(runCmd string, envMap map[string]string) string {
	return mimiVarRegex.ReplaceAllStringFunc(runCmd, func(m string) string {
		var varName string
		if strings.HasPrefix(m, "${") {
			varName = m[2 : len(m)-1]
		} else {
			varName = m[1:]
		}

		if val, ok := envMap[varName]; ok {
			return val
		}

		return m
	})
}
