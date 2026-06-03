package hooks

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/y3owk1n/mimi/internal/config"
	"github.com/y3owk1n/mimi/internal/events"
)

// Executor receives events, matches hooks, and runs shell commands.
type Executor struct {
	registry *Registry
	cfg      *config.SettingsConfig
	logger   *zap.SugaredLogger
	sem      chan struct{}
	cfgMu    sync.RWMutex
}

// NewExecutor creates an executor with the given registry and settings.
func NewExecutor(reg *Registry, cfg *config.SettingsConfig, logger *zap.SugaredLogger) *Executor {
	return &Executor{
		registry: reg,
		cfg:      cfg,
		logger:   logger,
		sem:      make(chan struct{}, cfg.MaxHookWorkers),
	}
}

// UpdateSettings hot-reloads the settings at runtime.
func (ex *Executor) UpdateSettings(cfg *config.SettingsConfig) {
	ex.cfgMu.Lock()
	ex.cfg = cfg
	ex.cfgMu.Unlock()
}

// Handle processes a single event and runs matching hooks.
func (ex *Executor) Handle(evt events.Event) {
	hooks := ex.registry.HooksFor(evt.Kind)
	ex.logger.Debugw(
		"processing event",
		"kind",
		evt.Kind,
		"event_id",
		evt.ID,
		"hooks_registered",
		len(hooks),
	)

	for _, hook := range hooks {
		matched, reason := hook.Matches(evt)
		if !matched {
			ex.logger.Debugw(
				"hook skipped",
				"kind",
				evt.Kind,
				"cmd",
				hook.Entry.Run,
				"reason",
				reason,
			)

			continue
		}

		ex.logger.Debugw(
			"hook matched",
			"kind",
			evt.Kind,
			"cmd",
			hook.Entry.Run,
			"async",
			hook.Entry.Async,
		)

		if hook.Entry.Async {
			go ex.run(hook, evt)
		} else {
			ex.run(hook, evt)
		}
	}
}

// Run reads events from the subscriber channel and handles them.
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

func (ex *Executor) run(hook Hook, evt events.Event) {
	ex.sem <- struct{}{}
	defer func() { <-ex.sem }()

	ex.cfgMu.RLock()
	shell := ex.cfg.HookShell
	timeout := time.Duration(ex.cfg.HookTimeoutSecs) * time.Second
	ex.cfgMu.RUnlock()

	if hook.Entry.TimeoutSecs > 0 {
		timeout = time.Duration(hook.Entry.TimeoutSecs) * time.Second
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	runCmd := replaceEventVars(hook.Entry.Run, eventEnvMap(evt))
	cmd := exec.CommandContext(ctx, shell, "-c", runCmd)

	cmd.Env = append(os.Environ(), eventEnv(evt)...)

	start := time.Now()
	out, err := cmd.CombinedOutput()
	elapsed := time.Since(start)

	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			ex.logger.Warnw("hook timed out",
				"kind", evt.Kind, "cmd", hook.Entry.Run, "timeout", timeout)
		} else {
			ex.logger.Errorw("hook failed",
				"kind", evt.Kind, "cmd", hook.Entry.Run,
				"exit", err, "output", strings.TrimSpace(string(out)))
		}
	} else {
		output := strings.TrimSpace(string(out))

		attrs := []any{
			"kind", evt.Kind,
			"cmd", hook.Entry.Run,
			"elapsed", elapsed.Round(time.Millisecond),
		}
		if output != "" {
			attrs = append(attrs, "output", output)
		}

		ex.logger.Debugw("hook ok", attrs...)
	}
}

const baseEnvVarCount = 9

func eventEnv(evt events.Event) []string {
	vars := make([]string, 0, baseEnvVarCount+len(evt.Extra))

	vars = append(vars,
		fmt.Sprintf("mimi_EVENT=%s", evt.Kind),
		"mimi_EVENT_ID="+evt.ID,
		"mimi_APP_NAME="+evt.AppName,
		"mimi_BUNDLE_ID="+evt.BundleID,
		fmt.Sprintf("mimi_PID=%d", evt.PID),
		"mimi_WINDOW_TITLE="+evt.WindowTitle,
		"mimi_VOLUME_PATH="+evt.VolumePath,
		"mimi_VOLUME_NAME="+evt.VolumeName,
		"mimi_TIMESTAMP="+evt.At.Format(time.RFC3339),
	)
	for k, v := range evt.Extra {
		vars = append(vars, fmt.Sprintf("mimi_%s=%s", strings.ToUpper(k), v))
	}

	return vars
}

func eventEnvMap(e events.Event) map[string]string {
	env := eventEnv(e)

	envMap := make(map[string]string, len(env))
	for _, kv := range env {
		parts := strings.SplitN(kv, "=", 2) //nolint:mnd
		if len(parts) == 2 {                //nolint:mnd
			envMap[parts[0]] = parts[1]
		}
	}

	return envMap
}

var mimiVarRegex = regexp.MustCompile(`\${mimi_[A-Za-z0-9_]+}|\$mimi_[A-Za-z0-9_]+`)

func replaceEventVars(runCmd string, envMap map[string]string) string {
	return mimiVarRegex.ReplaceAllStringFunc(runCmd, func(match string) string {
		var varName string
		if strings.HasPrefix(match, "${") {
			varName = match[2 : len(match)-1]
		} else {
			varName = match[1:]
		}

		if val, ok := envMap[varName]; ok {
			return val
		}

		return match
	})
}
