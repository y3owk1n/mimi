package cmd

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	derrors "github.com/y3owk1n/mimi/internal/errors"
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
		return tailEventLog(cmd, jsonFlag, kindFilter, appFilter)
	},
}

func init() {
	eventsCmd.Flags().BoolVar(&jsonFlag, "json", false, "output JSON lines")
	eventsCmd.Flags().StringVar(&kindFilter, "kind", "", "filter by event kind")
	eventsCmd.Flags().StringVar(&appFilter, "app", "", "filter by app name")
}

func tailEventLog(cmd *cobra.Command, jsonOut bool, kind, app string) error {
	eventLogPath := expandHome("~/.local/share/mimi/mimi.log.events.jsonl")

	eventFile, err := os.Open(eventLogPath)
	if err != nil {
		// If the file doesn't exist yet, create it and start tailing
		if os.IsNotExist(err) {
			err := os.MkdirAll(filepath.Dir(eventLogPath), 0o755) //nolint:mnd
			if err != nil {
				return err
			}

			eventFile, err = os.Create(eventLogPath)
			if err != nil {
				return derrors.Wrapf(err, derrors.CodeLoggingFailed, "creating event log")
			}
		} else {
			return derrors.Wrapf(err, derrors.CodeLoggingFailed, "opening event log")
		}
	}

	// Seek to end for tailing
	_, _ = eventFile.Seek(0, io.SeekEnd)

	scanner := bufio.NewScanner(eventFile)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var evt events.Event

		err := json.Unmarshal([]byte(line), &evt)
		if err != nil {
			continue
		}

		if kind != "" && string(evt.Kind) != kind {
			continue
		}

		if app != "" && !strings.Contains(strings.ToLower(evt.AppName), strings.ToLower(app)) {
			continue
		}

		if jsonOut {
			cmd.Println(line)
		} else {
			cmd.Printf("%s | %s", evt.At.Format("15:04:05"), evt.Kind)

			if evt.AppName != "" {
				cmd.Printf(" | %s", evt.AppName)
			}

			if evt.BundleID != "" {
				cmd.Printf(" (%s)", evt.BundleID)
			}

			if evt.WindowTitle != "" {
				cmd.Printf(" | \"%s\"", evt.WindowTitle)
			}

			cmd.Println()
		}
	}

	return scanner.Err()
}
