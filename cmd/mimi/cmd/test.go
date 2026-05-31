package cmd

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"github.com/y3owk1n/mimi/internal/config"
	"github.com/y3owk1n/mimi/internal/events"
	"github.com/y3owk1n/mimi/internal/hooks"
	"github.com/y3owk1n/mimi/internal/logging"
)

var (
	testApp    string
	testBundle string
	testTitle  string
)

var testCmd = &cobra.Command{
	Use:   "test <event-kind>",
	Short: "Fire a synthetic event to test your hooks",
	Example: `  mimi test app_activate --app Slack
  mimi test window_focus --title "GitHub - Safari"
  mimi test system_sleep`,
	Args: cobra.ExactArgs(1),
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		var names []string
		for _, k := range events.AllKinds {
			names = append(names, string(k))
		}
		return names, cobra.ShellCompDirectiveNoFileComp
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		kind := events.EventKind(args[0])
		e := events.Event{
			ID:          uuid.NewString(),
			Kind:        kind,
			AppName:     testApp,
			BundleID:    testBundle,
			WindowTitle: testTitle,
			At:          time.Now(),
		}
		cfg, err := config.Load(configPath)
		if err != nil {
			return err
		}
		reg := hooks.NewRegistry()
		if err := reg.Reload(cfg); err != nil {
			return err
		}
		logger := logging.New(cfg)
		executor := hooks.NewExecutor(reg, &cfg.Settings, logger)
		fmt.Printf("Firing synthetic event: %s\n", kind)
		executor.Handle(e)
		return nil
	},
}

func init() {
	testCmd.Flags().StringVar(&testApp, "app", "", "app name for synthetic event")
	testCmd.Flags().StringVar(&testBundle, "bundle", "", "bundle ID for synthetic event")
	testCmd.Flags().StringVar(&testTitle, "title", "", "window title for synthetic event")
}
