package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/y3owk1n/mimi/internal/events"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show daemon status and recent events",
	RunE: func(cmd *cobra.Command, args []string) error {
		pid, err := readPID(defaultPIDPath())
		if err != nil {
			fmt.Println("mimi: not running")
			return nil
		}
		proc, err := os.FindProcess(pid)
		running := err == nil && proc.Signal(syscall.Signal(0)) == nil
		if running {
			fmt.Printf("mimi: running (pid %d)\n", pid)
		} else {
			fmt.Println("mimi: not running (stale PID file)")
		}
		printRecentEvents(10)
		return nil
	},
}

func printRecentEvents(n int) {
	eventLogPath := expandHome("~/.local/share/mimi/mimi.log.events.jsonl")
	data, err := os.ReadFile(eventLogPath)
	if err != nil {
		return
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) == 0 {
		return
	}
	start := len(lines) - n
	if start < 0 {
		start = 0
	}
	fmt.Println("\nRecent events:")
	for _, line := range lines[start:] {
		var e events.Event
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			continue
		}
		fmt.Printf("  %s | %s", e.At.Format("15:04:05"), e.Kind)
		if e.AppName != "" {
			fmt.Printf(" | %s", e.AppName)
		}
		if e.BundleID != "" {
			fmt.Printf(" (%s)", e.BundleID)
		}
		fmt.Println()
	}
}
