package cmd

import (
	"encoding/json"
	"os"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/y3owk1n/mimi/internal/events"
)

const recentEventsCount = 10

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show daemon status and recent events",
	RunE: func(cmd *cobra.Command, args []string) error {
		pid, err := readPID(defaultPIDPath())
		if err != nil {
			cmd.Println("mimi: not running")

			return nil
		}

		proc, err := os.FindProcess(pid)

		running := err == nil && proc.Signal(syscall.Signal(0)) == nil
		if running {
			cmd.Printf("mimi: running (pid %d)\n", pid)
		} else {
			cmd.Println("mimi: not running (stale PID file)")
		}

		printRecentEvents(cmd, recentEventsCount)

		return nil
	},
}

func printRecentEvents(cmd *cobra.Command, count int) {
	eventLogPath := expandHome("~/.local/share/mimi/mimi.log.events.jsonl")

	data, err := os.ReadFile(eventLogPath)
	if err != nil {
		return
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) == 0 {
		return
	}

	start := max(len(lines)-count, 0)

	cmd.Println("\nRecent events:")

	for _, line := range lines[start:] {
		var evt events.Event

		err := json.Unmarshal([]byte(line), &evt)
		if err != nil {
			continue
		}

		cmd.Printf("  %s | %s", evt.At.Format("15:04:05"), evt.Kind)

		if evt.AppName != "" {
			cmd.Printf(" | %s", evt.AppName)
		}

		if evt.BundleID != "" {
			cmd.Printf(" (%s)", evt.BundleID)
		}

		cmd.Println()
	}
}
