package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"github.com/y3owk1n/mimi/internal/config"
	derrors "github.com/y3owk1n/mimi/internal/errors"
	"github.com/y3owk1n/mimi/internal/events"
	"github.com/y3owk1n/mimi/internal/hooks"
)

func newHooksCmd(state *cliState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "hooks",
		Short: "List the hooks in the config, or fire one kind by hand",
		Long: `Read the hooks the config binds, and run the hooks of one kind against an
event you describe, in this process, the way the daemon would run them.

  mimi hooks list
  mimi hooks fire on_app_activate --app Safari`,
		RunE: func(cobraCmd *cobra.Command, _ []string) error {
			cobraCmd.SilenceUsage = false

			return derrors.New(
				derrors.CodeInvalidInput,
				"hooks subcommand required (mimi hooks list, mimi hooks fire)",
			)
		},
	}

	cmd.AddCommand(newHooksListCmd(state))
	cmd.AddCommand(newHooksFireCmd(state))

	return cmd
}

func newHooksListCmd(state *cliState) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List every hook the config binds, by kind",
		Long: `Print each hook kind that has at least one hook, with every entry under it:
its position, its command, and the filters and settings on it. A config with
no hooks prints nothing and exits 0. mimi reads the config from disk, so
this shows what a reload would load, not what a running daemon holds.`,
		Args: cobra.NoArgs,
		RunE: func(cobraCmd *cobra.Command, _ []string) error {
			cfg, err := config.Load(state.configPath)
			if err != nil {
				return derrors.Wrapf(err, derrors.CodeInvalidConfig, "loading config")
			}

			cobraCmd.Print(formatHookList(cfg))

			return nil
		},
	}
}

// formatHookList renders the hooks a config binds, one kind per block.
func formatHookList(cfg *config.Config) string {
	var out strings.Builder

	for _, kind := range config.HookKinds {
		entries := *kind.Entries(&cfg.Hooks)
		if len(entries) == 0 {
			continue
		}

		fmt.Fprintf(&out, "%s\n", kind.TOMLKey)

		for index, entry := range entries {
			fmt.Fprintf(&out, "  [%d] %s\n", index, entry.Run)

			if settings := entrySettings(entry); settings != "" {
				fmt.Fprintf(&out, "      %s\n", settings)
			}
		}
	}

	return out.String()
}

// entrySettings is the filters and settings on one entry, "" for none.
func entrySettings(entry config.HookEntry) string {
	var parts []string

	if entry.App != "" {
		parts = append(parts, "app="+entry.App)
	}

	if entry.BundleID != "" {
		parts = append(parts, "bundle_id="+entry.BundleID)
	}

	if entry.Title != "" {
		parts = append(parts, "title="+entry.Title)
	}

	if entry.Space != "" {
		parts = append(parts, "space="+entry.Space)
	}

	if entry.TimeoutSecs > 0 {
		parts = append(parts, fmt.Sprintf("timeout_secs=%d", entry.TimeoutSecs))
	}

	if entry.Async {
		parts = append(parts, "async")
	}

	return strings.Join(parts, " ")
}

// The flags mimi hooks fire takes to describe the event.
const (
	fireAppFlag    = "app"
	fireBundleFlag = "bundle-id"
	firePIDFlag    = "pid"
	fireTitleFlag  = "title"
	fireExtraFlag  = "extra"
)

func newHooksFireCmd(state *cliState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "fire <kind>",
		Short: "Run the hooks of one kind against an event you describe",
		Long: `Build an event of the given kind from the flags and run the kind's hooks
against it in this process, one after another, the way the daemon runs them:
through settings.hook_shell, with the mimi_* variables set and the event on
stdin, under the hook's timeout. The command reports every hook, matched or
not:

  $ mimi hooks fire on_app_activate --app Safari
  [0] matched, ok in 12ms
      active: Safari
  [1] skipped: app filter mismatch

The kind is a key from [hooks], such as on_app_activate, or the event name
the daemon logs, such as app_activate. --extra sets a variable a kind would
carry, as key=value: --extra space_index=2 for on_workspace_changed, say.
Exits 1 when a matched hook fails or times out. No daemon is needed, and a
running one is not involved.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cobraCmd *cobra.Command, args []string) error {
			kind, ok := hookKindNamed(args[0])
			if !ok {
				return derrors.Newf(
					derrors.CodeInvalidInput,
					"unknown hook kind %q (recognized: %s)",
					args[0],
					strings.Join(config.HookKindNames(), ", "),
				)
			}

			cfg, err := config.Load(state.configPath)
			if err != nil {
				return derrors.Wrapf(err, derrors.CodeInvalidConfig, "loading config")
			}

			evt, err := eventFromFlags(cobraCmd, kind)
			if err != nil {
				return err
			}

			reg := hooks.NewRegistry()

			err = reg.Reload(cfg)
			if err != nil {
				return derrors.Wrapf(err, derrors.CodeInvalidConfig, "compiling hook filters")
			}

			outcomes := hooks.NewExecutor(reg, &cfg.Settings, nil).Fire(cobraCmd.Context(), evt)
			cobraCmd.Print(formatOutcomes(outcomes))

			for _, outcome := range outcomes {
				if outcome.Matched && outcome.Result.Err != nil {
					return derrors.New(derrors.CodeActionFailed, "a hook failed")
				}
			}

			return nil
		},
	}

	cmd.Flags().String(fireAppFlag, "", "the application's name")
	cmd.Flags().String(fireBundleFlag, "", "the application's bundle identifier")
	cmd.Flags().Int(firePIDFlag, 0, "the application's process id")
	cmd.Flags().String(fireTitleFlag, "", "the window's title")
	cmd.Flags().StringArray(fireExtraFlag, nil, "an extra variable, as key=value (repeatable)")

	return cmd
}

// hookKindNamed resolves a [hooks] key or an event name to its event kind.
func hookKindNamed(name string) (events.EventKind, bool) {
	for _, kind := range config.HookKinds {
		if name == kind.TOMLKey || name == string(kind.Kind) {
			return kind.Kind, true
		}
	}

	return "", false
}

// eventFromFlags is the synthetic event the flags describe.
func eventFromFlags(cobraCmd *cobra.Command, kind events.EventKind) (events.Event, error) {
	flags := cobraCmd.Flags()
	app, _ := flags.GetString(fireAppFlag)
	bundle, _ := flags.GetString(fireBundleFlag)
	pid, _ := flags.GetInt(firePIDFlag)
	title, _ := flags.GetString(fireTitleFlag)
	extras, _ := flags.GetStringArray(fireExtraFlag)

	evt := events.Event{
		ID:          uuid.NewString(),
		Kind:        kind,
		AppName:     app,
		BundleID:    bundle,
		PID:         pid,
		WindowTitle: title,
		At:          time.Now(),
	}

	for _, extra := range extras {
		key, value, found := strings.Cut(extra, "=")
		if !found || key == "" {
			return events.Event{}, derrors.Newf(
				derrors.CodeInvalidInput,
				"--extra takes key=value, got %q",
				extra,
			)
		}

		if evt.Extra == nil {
			evt.Extra = map[string]string{}
		}

		evt.Extra[key] = value
	}

	return evt, nil
}

// formatOutcomes renders what Fire reported, one hook per line with its
// output indented under it.
func formatOutcomes(outcomes []hooks.Outcome) string {
	if len(outcomes) == 0 {
		return "no hooks of that kind\n"
	}

	var out strings.Builder

	for _, outcome := range outcomes {
		if !outcome.Matched {
			fmt.Fprintf(&out, "[%d] skipped: %s\n", outcome.Index, outcome.Reason)

			continue
		}

		result := outcome.Result
		elapsed := result.Elapsed.Round(time.Millisecond)

		switch {
		case result.TimedOut:
			fmt.Fprintf(&out, "[%d] matched, timed out after %s\n", outcome.Index, result.Timeout)
		case result.Err != nil:
			fmt.Fprintf(
				&out,
				"[%d] matched, failed in %s: %v\n",
				outcome.Index,
				elapsed,
				result.Err,
			)
		default:
			fmt.Fprintf(&out, "[%d] matched, ok in %s\n", outcome.Index, elapsed)
		}

		output := strings.TrimRight(string(result.Output), "\n")
		if output == "" {
			continue
		}

		for line := range strings.SplitSeq(output, "\n") {
			fmt.Fprintf(&out, "    %s\n", line)
		}
	}

	return out.String()
}
