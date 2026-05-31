package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/spf13/cobra"
)

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Install mimi as a launchd user agent (auto-starts at login)",
	RunE: func(cmd *cobra.Command, args []string) error {
		binaryPath, err := os.Executable()
		if err != nil {
			return err
		}
		home := os.Getenv("HOME")
		plistPath := filepath.Join(home, "Library/LaunchAgents/com.y3owk1n.mimi.plist")

		var buf strings.Builder
		if err := plistTmpl.Execute(&buf, map[string]string{
			"BinaryPath": binaryPath,
			"ConfigPath": configPath,
			"Home":       home,
		}); err != nil {
			return fmt.Errorf("rendering plist: %w", err)
		}

		if err := os.MkdirAll(filepath.Dir(plistPath), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(plistPath, []byte(buf.String()), 0o644); err != nil {
			return fmt.Errorf("writing plist: %w", err)
		}

		out, err := exec.Command("launchctl", "load", "-w", plistPath).CombinedOutput()
		if err != nil {
			return fmt.Errorf("launchctl load: %s: %w", out, err)
		}
		fmt.Printf("mimi installed as launchd agent.\nPlist: %s\n", plistPath)
		return nil
	},
}

var uninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove the launchd agent",
	RunE: func(cmd *cobra.Command, args []string) error {
		home := os.Getenv("HOME")
		plistPath := filepath.Join(home, "Library/LaunchAgents/com.y3owk1n.mimi.plist")
		exec.Command("launchctl", "unload", plistPath).Run()
		os.Remove(plistPath)
		fmt.Println("mimi uninstalled.")
		return nil
	},
}

var plistTmpl = template.Must(template.New("plist").Parse(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
  "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.y3owk1n.mimi</string>

    <key>ProgramArguments</key>
    <array>
        <string>{{.BinaryPath}}</string>
        <string>start</string>
        <string>--config</string>
        <string>{{.ConfigPath}}</string>
    </array>

    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>

    <key>ThrottleInterval</key>
    <integer>5</integer>

    <key>StandardOutPath</key>
    <string>{{.Home}}/.local/share/mimi/stdout.log</string>
    <key>StandardErrorPath</key>
    <string>{{.Home}}/.local/share/mimi/stderr.log</string>

    <key>ProcessType</key>
    <string>Interactive</string>
</dict>
</plist>`))
