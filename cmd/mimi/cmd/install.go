package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/spf13/cobra"

	derrors "github.com/y3owk1n/mimi/internal/errors"
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

		err = plistTmpl.Execute(&buf, map[string]string{
			"BinaryPath": binaryPath,
			"ConfigPath": configPath,
			"Home":       home,
		})
		if err != nil {
			return derrors.Wrapf(err, derrors.CodeInternal, "rendering plist")
		}

		err = os.MkdirAll(filepath.Dir(plistPath), 0o755) //nolint:mnd
		if err != nil {
			return err
		}

		err = os.WriteFile(plistPath, []byte(buf.String()), 0o644) //nolint:mnd
		if err != nil {
			return derrors.Wrapf(err, derrors.CodeConfigIOFailed, "writing plist")
		}

		out, err := exec.Command("launchctl", "load", "-w", plistPath).CombinedOutput()
		if err != nil {
			return derrors.Wrapf(err, derrors.CodeInternal, "launchctl load: %s", out)
		}

		cmd.Printf("mimi installed as launchd agent.\nPlist: %s\n", plistPath)

		return nil
	},
}

var uninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove the launchd agent",
	RunE: func(cmd *cobra.Command, args []string) error {
		home := os.Getenv("HOME")
		plistPath := filepath.Join(home, "Library/LaunchAgents/com.y3owk1n.mimi.plist")
		_ = exec.Command("launchctl", "unload", plistPath).Run()
		_ = os.Remove(plistPath)

		cmd.Println("mimi uninstalled.")

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
