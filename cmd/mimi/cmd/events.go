package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/y3owk1n/mimi/internal/events"
)

var (
	jsonFlag   bool
	kindFilter string
	appFilter  string
)

var eventsCmd = &cobra.Command{
	Use:   "events",
	Short: "Tail the live event stream",
	Long: `Stream events to stdout as they occur.

  Flags:
    --json     output JSON lines instead of human-readable text
    --kind     filter to a specific event kind (e.g. --kind app_activate)
    --app      filter by app name glob
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return tailEventLog(jsonFlag, kindFilter, appFilter)
	},
}

func init() {
	eventsCmd.Flags().BoolVar(&jsonFlag, "json", false, "output JSON lines")
	eventsCmd.Flags().StringVar(&kindFilter, "kind", "", "filter by event kind")
	eventsCmd.Flags().StringVar(&appFilter, "app", "", "filter by app name")
}

func tailEventLog(jsonOut bool, kind, app string) error {
	eventLogPath := expandHome("~/.local/share/mimi/mimi.log.events.jsonl")

	f, err := os.Open(eventLogPath)
	if err != nil {
		// If the file doesn't exist yet, create it and start tailing
		if os.IsNotExist(err) {
			err := os.MkdirAll(filepath.Dir(eventLogPath), 0o755)
			if err != nil {
				return err
			}

			f, err = os.Create(eventLogPath)
			if err != nil {
				return fmt.Errorf("creating event log: %w", err)
			}
		} else {
			return fmt.Errorf("opening event log: %w", err)
		}
	}

	// Seek to end for tailing
	_, _ = f.Seek(0, 2)

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var e events.Event

		err := json.Unmarshal([]byte(line), &e)
		if err != nil {
			continue
		}

		if kind != "" && string(e.Kind) != kind {
			continue
		}

		if app != "" && !strings.Contains(strings.ToLower(e.AppName), strings.ToLower(app)) {
			continue
		}

		if jsonOut {
			fmt.Println(line)
		} else {
			fmt.Printf("%s | %s", e.At.Format("15:04:05"), e.Kind)

			if e.AppName != "" {
				fmt.Printf(" | %s", e.AppName)
			}

			if e.BundleID != "" {
				fmt.Printf(" (%s)", e.BundleID)
			}

			if e.WindowTitle != "" {
				fmt.Printf(" | \"%s\"", e.WindowTitle)
			}

			fmt.Println()
		}
	}

	return scanner.Err()
}
