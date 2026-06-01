package cmd

import (
	"encoding/json"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	derrors "github.com/y3owk1n/mimi/internal/errors"
	"github.com/y3owk1n/mimi/internal/events"
)

const pollInterval = 500 * time.Millisecond

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

	offset := int64(0)

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for range ticker.C {
		eventFile, err := os.Open(eventLogPath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}

			return derrors.Wrapf(err, derrors.CodeLoggingFailed, "opening event log")
		}

		stat, staterr := eventFile.Stat()
		if staterr != nil {
			_ = eventFile.Close()

			continue
		}

		if stat.Size() <= offset {
			_ = eventFile.Close()

			continue
		}

		_, seekerr := eventFile.Seek(offset, 0)
		if seekerr != nil {
			_ = eventFile.Close()

			continue
		}

		readBuf := make([]byte, stat.Size()-offset)
		_, readerr := eventFile.Read(readBuf)
		offset = stat.Size()
		_ = eventFile.Close()

		if readerr != nil {
			continue
		}

		lines := strings.SplitSeq(string(readBuf), "\n")
		for line := range lines {
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
	}

	return nil
}
