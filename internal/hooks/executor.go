package hooks

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/y3owk1n/mimi/internal/config"
	"github.com/y3owk1n/mimi/internal/events"
	"github.com/y3owk1n/mimi/internal/shellexec"
)

// maxHookOutputBytes caps the combined stdout+stderr capture for a single
// hook invocation. cmd.CombinedOutput() would otherwise allocate a buffer
// sized to whatever the child writes, which is unbounded for a misbehaving
// or chatty hook.
const maxHookOutputBytes = 64 * 1024

// hookOutputBuffer is a bytes.Buffer-backed writer that silently drops
// writes past maxHookOutputBytes so a misbehaving hook can't make the
// daemon allocate arbitrarily large buffers.
type hookOutputBuffer struct {
	buf   bytes.Buffer
	limit int
}

func (h *hookOutputBuffer) Write(
	data []byte,
) (int, error) {
	remaining := h.limit - h.buf.Len()
	if remaining <= 0 {
		return len(data), nil
	}

	if len(data) > remaining {
		h.buf.Write(data[:remaining])

		return len(data), nil
	}

	return h.buf.Write(data)
}

func (h *hookOutputBuffer) Bytes() []byte {
	return h.buf.Bytes()
}

// Executor receives events, matches hooks, and runs shell commands.
type Executor struct {
	registry *Registry
	cfg      *config.SettingsConfig
	logger   *zap.SugaredLogger
	sem      chan struct{}
	cfgMu    sync.RWMutex

	// baseEnv is the process environment captured at construction time and
	// reused as the base for every hook invocation. os.Environ() allocates
	// a new []string on every call, which is wasted work since the daemon's
	// environment is effectively constant for its lifetime.
	baseEnv []string
}

// NewExecutor creates an executor with the given registry and settings.
func NewExecutor(reg *Registry, cfg *config.SettingsConfig, logger *zap.SugaredLogger) *Executor {
	if logger == nil {
		logger = zap.NewNop().Sugar()
	}

	return &Executor{
		registry: reg,
		cfg:      cfg,
		logger:   logger,
		sem:      make(chan struct{}, cfg.MaxHookWorkers),
		baseEnv:  os.Environ(),
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

	for hookIndex, hook := range hooks {
		matched, reason := hook.Matches(evt)
		if !matched {
			ex.logger.Debugw(
				"hook skipped",
				"kind",
				evt.Kind,
				"index",
				hookIndex,
				"reason",
				reason,
			)

			continue
		}

		ex.logger.Debugw(
			"hook matched",
			"kind",
			evt.Kind,
			"index",
			hookIndex,
			"async",
			hook.Entry.Async,
		)

		if hook.Entry.Async {
			go ex.run(hookIndex, hook, evt)
		} else {
			ex.run(hookIndex, hook, evt)
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

// Outcome is what Fire reports for one hook of the kind: whether it matched
// the event and why not when it did not, and how it ran when it did.
type Outcome struct {
	Index   int
	Matched bool
	Reason  string
	Result  Result
}

// Fire runs the hooks for evt's kind by hand, one after another whatever
// their async setting, and reports each. It is how mimi hooks fire shows a
// user what the daemon would do with such an event.
func (ex *Executor) Fire(ctx context.Context, evt events.Event) []Outcome {
	hooks := ex.registry.HooksFor(evt.Kind)
	outcomes := make([]Outcome, 0, len(hooks))

	for index, hook := range hooks {
		outcome := Outcome{Index: index}

		outcome.Matched, outcome.Reason = hook.Matches(evt)
		if outcome.Matched {
			outcome.Result = ex.execute(ctx, hook, evt)
		}

		outcomes = append(outcomes, outcome)
	}

	return outcomes
}

// run executes a single matched hook and logs how it went. hookIndex is the
// hook's 0-based position within its kind, passed down from Handle's loop so
// the log lines below can identify the hook without recording its command
// text. It is a parameter rather than executor state because Handle may
// launch several async hooks concurrently.
func (ex *Executor) run(hookIndex int, hook Hook, evt events.Event) {
	result := ex.execute(context.Background(), hook, evt)

	switch {
	case result.TimedOut:
		ex.logger.Warnw("hook timed out",
			"kind", evt.Kind, "index", hookIndex, "timeout", result.Timeout)
		ex.logFailedOutput(hookIndex, evt, result)
	case result.Err != nil:
		ex.logger.Warnw("hook failed",
			"kind", evt.Kind, "index", hookIndex,
			"exit", result.Err)
		ex.logFailedOutput(hookIndex, evt, result)
	default:
		output := strings.TrimSpace(string(result.Output))

		attrs := []any{
			"kind", evt.Kind,
			"index", hookIndex,
			"elapsed", result.Elapsed.Round(time.Millisecond),
		}
		if output != "" {
			attrs = append(attrs, "output", output)
		}

		ex.logger.Debugw("hook ok", attrs...)
	}
}

// logFailedOutput logs what a failed hook printed, at debug only, since it is
// the hook's own output and the warning above it must not carry it.
func (ex *Executor) logFailedOutput(hookIndex int, evt events.Event, result Result) {
	output := strings.TrimSpace(string(result.Output))
	if output == "" {
		return
	}

	ex.logger.Debugw("hook output", "kind", evt.Kind, "index", hookIndex, "output", output)
}

// Result is how one hook run went: what it printed, capped as the daemon
// caps it, how long it took, the bound it ran under, and why it failed when
// it did.
type Result struct {
	Output   []byte
	Elapsed  time.Duration
	Timeout  time.Duration
	TimedOut bool
	Err      error
}

// execute runs one hook the way the daemon does, under its timeout and at
// most max_hook_workers at once, and reports the result rather than logging
// it. parent bounds the run beyond the timeout, for a caller that may be
// interrupted.
func (ex *Executor) execute(parent context.Context, hook Hook, evt events.Event) Result {
	ex.sem <- struct{}{}
	defer func() { <-ex.sem }()

	ex.cfgMu.RLock()
	shell := ex.cfg.HookShell
	timeout := time.Duration(ex.cfg.HookTimeoutSecs) * time.Second
	ex.cfgMu.RUnlock()

	if hook.Entry.TimeoutSecs > 0 {
		timeout = time.Duration(hook.Entry.TimeoutSecs) * time.Second
	}

	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	runCmd := replaceEventVars(hook.Entry.Run, evt)
	cmd := shellexec.Command(ctx, shell, runCmd)
	cmd.Stdin = bytes.NewReader(eventJSON(evt))

	eventVars := eventEnv(evt)
	cmd.Env = make([]string, 0, len(ex.baseEnv)+len(eventVars))
	cmd.Env = append(cmd.Env, ex.baseEnv...)
	cmd.Env = append(cmd.Env, eventVars...)

	start := time.Now()
	outBuf := &hookOutputBuffer{limit: maxHookOutputBytes}
	cmd.Stdout = outBuf
	cmd.Stderr = outBuf
	err := shellexec.Run(cmd)

	return Result{
		Output:   outBuf.Bytes(),
		Elapsed:  time.Since(start),
		Timeout:  timeout,
		TimedOut: err != nil && errors.Is(ctx.Err(), context.DeadlineExceeded),
		Err:      err,
	}
}

const baseEnvVarCount = 8

// eventJSON is the event as one line of JSON, the same document the event
// log writes, for the hook's stdin. A hook that wants the whole event
// reads it with jq rather than assembling it from the variables.
func eventJSON(evt events.Event) []byte {
	data, err := json.Marshal(evt)
	if err != nil {
		return nil
	}

	return append(data, '\n')
}

func eventEnv(evt events.Event) []string {
	vars := make([]string, 0, baseEnvVarCount+len(evt.Extra))

	vars = append(vars,
		fmt.Sprintf("mimi_EVENT=%s", evt.Kind),
		"mimi_EVENT_ID="+evt.ID,
		"mimi_APP_NAME="+evt.AppName,
		"mimi_BUNDLE_ID="+evt.BundleID,
		"mimi_PID="+pid(evt),
		"mimi_WINDOW_TITLE="+evt.WindowTitle,
		"mimi_WINDOW_NUMBER="+windowNumber(evt),
		"mimi_TIMESTAMP="+evt.At.Format(time.RFC3339),
	)
	for k, v := range evt.Extra {
		vars = append(vars, fmt.Sprintf("mimi_%s=%s", strings.ToUpper(k), v))
	}

	return vars
}

// pid is mimi_PID, empty when the event names no application.
func pid(evt events.Event) string {
	if evt.PID == 0 {
		return ""
	}

	return strconv.Itoa(evt.PID)
}

// windowNumber is mimi_WINDOW_NUMBER, empty when the event names no window.
func windowNumber(evt events.Event) string {
	if evt.WindowNumber == 0 {
		return ""
	}

	return strconv.FormatUint(uint64(evt.WindowNumber), 10)
}

var mimiVarRegex = regexp.MustCompile(`^(?:\$\{(mimi_[A-Za-z0-9_]+)\}|\$(mimi_[A-Za-z0-9_]+))`)

// replaceEventVars rewrites each $mimi_FOO and ${mimi_FOO} reference in runCmd
// so the shell expands it from the hook's environment, where eventEnv exports
// every value. No value enters the command text. A window title is whatever a
// web page chose to call itself, and it cannot run as code whatever quotes
// the user put around the reference.
//
// Each rewrite matches the quotes around the reference, so it still expands
// to the value as one word. Outside quotes it becomes "$mimi_FOO". Inside
// single quotes it becomes '"$mimi_FOO"', which closes the user's quote and
// reopens it. Inside double quotes the reference stays as written. If the
// scan misreads the quotes, the hook prints the wrong text but runs nothing
// new.
func replaceEventVars(runCmd string, evt events.Event) string {
	var out strings.Builder

	var quote byte

	for idx := 0; idx < len(runCmd); idx++ {
		char := runCmd[idx]

		switch {
		case char == '\\' && quote != '\'' && idx+1 < len(runCmd):
			out.WriteString(runCmd[idx : idx+2])
			idx++

			continue
		case (char == '\'' || char == '"') && (quote == 0 || quote == char):
			if quote == 0 {
				quote = char
			} else {
				quote = 0
			}
		case char == '$':
			match := mimiVarRegex.FindStringSubmatch(runCmd[idx:])
			if match == nil {
				break
			}

			name := match[1] + match[2]

			envName, ok := eventEnvName(name, evt)
			if !ok {
				break
			}

			out.WriteString(envReference(envName, quote, runCmd[idx+len(match[0]):]))
			idx += len(match[0]) - 1

			continue
		}

		out.WriteByte(char)
	}

	return out.String()
}

// envReference expands the environment variable name as one word inside
// quote. rest is the command text after the reference, which decides whether
// a reference in double quotes needs braces.
func envReference(name string, quote byte, rest string) string {
	switch quote {
	case '\'':
		return `'"$` + name + `"'`
	case '"':
		if rest != "" && isNameChar(rest[0]) {
			return "${" + name + "}"
		}

		return "$" + name
	default:
		return `"$` + name + `"`
	}
}

func isNameChar(char byte) bool {
	return char == '_' || ('0' <= char && char <= '9') ||
		('a' <= char && char <= 'z') || ('A' <= char && char <= 'Z')
}

// eventEnvName is the name eventEnv exports a reference's value under, or
// false when name is not one of the event's variables. Extra variables match
// in any case, because eventEnv upper-cases their keys.
func eventEnvName(name string, evt events.Event) (string, bool) {
	switch name {
	case "mimi_EVENT", "mimi_EVENT_ID", "mimi_APP_NAME", "mimi_BUNDLE_ID",
		"mimi_PID", "mimi_WINDOW_TITLE", "mimi_WINDOW_NUMBER", "mimi_TIMESTAMP":
		return name, true
	}

	extraKey := strings.ToLower(strings.TrimPrefix(name, "mimi_"))
	if _, ok := evt.Extra[extraKey]; ok {
		return "mimi_" + strings.ToUpper(extraKey), true
	}

	return "", false
}
