package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

// servicesCmd manages the system service used for automatic startup.
//
// macOS: backed by launchd.
// Other platforms: stubbed and returns CodeNotSupported until implemented.
var servicesCmd = &cobra.Command{
	Use:   "services",
	Short: "Manage the Mimi system service (macOS launchd)",
	Long: `Manage the Mimi system service for automatic startup on login.

On macOS, this manages a launchd plist so Mimi starts automatically
when you log in. Available on macOS only.

Subcommands:
  install     Install and load the system service
  uninstall   Unload and remove the system service
  start       Start the system service
  stop        Stop the system service
  restart     Restart the system service
  status      Check whether the service is loaded and running`,
}

var servicesInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install and load the system service",
	Long:  "Install the Mimi launchd service so it starts automatically on login. Creates the plist file and loads it with launchctl.",
	RunE: func(cmd *cobra.Command, args []string) error {
		err := installService()
		if err != nil {
			return err
		}

		cmd.Println("Service installed and loaded successfully")

		return nil
	},
}

var servicesUninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Unload and remove the system service",
	Long:  "Unload the Mimi launchd service and remove its plist file. Mimi will no longer start automatically on login.",
	RunE: func(cmd *cobra.Command, args []string) error {
		err := uninstallService()
		if err != nil {
			return err
		}

		cmd.Println("Service uninstalled successfully")

		return nil
	},
}

var servicesStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the system service",
	Long:  "Start the Mimi launchd service. The daemon will begin running in the background.",
	RunE: func(cmd *cobra.Command, args []string) error {
		err := startService()
		if err != nil {
			return err
		}

		cmd.Println("Service started")

		return nil
	},
}

var servicesStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the system service",
	Long:  "Stop the Mimi launchd service. The daemon process will be terminated.",
	RunE: func(cmd *cobra.Command, args []string) error {
		err := stopService()
		if err != nil {
			return err
		}

		cmd.Println("Service stopped")

		return nil
	},
}

var servicesRestartCmd = &cobra.Command{
	Use:   "restart",
	Short: "Restart the system service",
	Long:  "Stop then immediately start the Mimi launchd service. Useful after configuration changes or to recover from an unresponsive state.",
	RunE: func(cmd *cobra.Command, args []string) error {
		err := restartService()
		if err != nil {
			return err
		}

		cmd.Println("Service restarted")

		return nil
	},
}

var servicesStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check the status of the system service",
	Long:  "Check whether the Mimi launchd service is currently loaded and running. Displays whether the service is active.",
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.Println(statusService())

		return nil
	},
}

func init() {
	addConfigPreRun(servicesInstallCmd)
	servicesCmd.AddCommand(servicesInstallCmd)
	servicesCmd.AddCommand(servicesUninstallCmd)
	servicesCmd.AddCommand(servicesStartCmd)
	servicesCmd.AddCommand(servicesStopCmd)
	servicesCmd.AddCommand(servicesRestartCmd)
	servicesCmd.AddCommand(servicesStatusCmd)
}

const (
	serviceLabel     = "com.y3owk1n.mimi"
	launchAgentsDir  = "~/Library/LaunchAgents"
	servicePlistFile = launchAgentsDir + "/" + serviceLabel + ".plist"
)

const servicePlistTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.y3owk1n.mimi</string>
    <key>ProgramArguments</key>
    <array>
        <string>MIMI_BINARY_PATH</string>
        <string>start</string>
        <string>--config</string>
        <string>MIMI_CONFIG_PATH</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>/tmp/mimi.log</string>
    <key>StandardErrorPath</key>
    <string>/tmp/mimi.err.log</string>
    <key>ProcessType</key>
    <string>Interactive</string>
    <key>LimitLoadToSessionType</key>
    <string>Aqua</string>
    <key>Nice</key>
    <integer>-10</integer>
    <key>ThrottleInterval</key>
    <integer>10</integer>
    <key>EnvironmentVariables</key>
    <dict>
        <key>PATH</key>
        <string>/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin</string>
    </dict>
</dict>
</plist>`

var (
	errServiceAlreadyLoaded = errors.New(
		"service is already loaded; check for existing installations (e.g., nix-darwin, home-manager) and uninstall them first",
	)
	errPlistAlreadyExists = errors.New("plist file already exists")
)

func getBinaryPath() (string, error) {
	execPath, err := os.Executable()
	if err != nil {
		return "", err
	}

	return filepath.EvalSymlinks(execPath)
}

func isServiceLoaded() bool {
	cmd := exec.CommandContext(context.Background(), "launchctl", "list", serviceLabel)

	return cmd.Run() == nil
}

func installService() error {
	if isServiceLoaded() {
		return errServiceAlreadyLoaded
	}

	binPath, err := getBinaryPath()
	if err != nil {
		return fmt.Errorf("failed to get binary path: %w", err)
	}

	plistContent := strings.ReplaceAll(servicePlistTemplate, "MIMI_BINARY_PATH", binPath)
	plistContent = strings.ReplaceAll(plistContent, "MIMI_CONFIG_PATH", configPath)

	expandedDir, err := expandPath(launchAgentsDir)
	if err != nil {
		return fmt.Errorf("failed to expand LaunchAgents path: %w", err)
	}

	const dirPerm = 0o755

	err = os.MkdirAll(expandedDir, dirPerm)
	if err != nil {
		return fmt.Errorf("failed to create LaunchAgents directory: %w", err)
	}

	expandedPlist := filepath.Join(expandedDir, serviceLabel+".plist")

	const filePerm = 0o644

	// Use O_EXCL to atomically create the file, avoiding TOCTOU between Stat and WriteFile.
	plistFile, err := os.OpenFile(expandedPlist, os.O_WRONLY|os.O_CREATE|os.O_EXCL, filePerm)
	if err != nil {
		if os.IsExist(err) {
			return fmt.Errorf(
				"%w at %s; remove it manually or uninstall first",
				errPlistAlreadyExists,
				expandedPlist,
			)
		}

		return fmt.Errorf("failed to create plist: %w", err)
	}

	_, err = plistFile.WriteString(plistContent)
	if err != nil {
		_ = plistFile.Close()
		_ = os.Remove(expandedPlist)

		return fmt.Errorf("failed to write plist: %w", err)
	}

	err = plistFile.Close()
	if err != nil {
		return fmt.Errorf("failed to close plist: %w", err)
	}

	currentUser, err := user.Current()
	if err != nil {
		return fmt.Errorf("failed to get current user: %w", err)
	}

	cmd := exec.CommandContext(
		context.Background(),
		"launchctl",
		"bootstrap",
		"gui/"+currentUser.Uid,
		expandedPlist,
	)

	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("failed to load service: %w", err)
	}

	return nil
}

func uninstallService() error {
	expandedPlist, err := expandPath(servicePlistFile)
	if err != nil {
		return fmt.Errorf("failed to expand plist path: %w", err)
	}

	currentUser, err := user.Current()
	if err != nil {
		return fmt.Errorf("failed to get current user: %w", err)
	}

	cmd := exec.CommandContext(
		context.Background(),
		"launchctl",
		"bootout",
		"gui/"+currentUser.Uid+"/"+serviceLabel,
	)
	_ = cmd.Run()

	err = os.Remove(expandedPlist)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove plist: %w", err)
	}

	return nil
}

func startService() error {
	cmd := exec.CommandContext(context.Background(), "launchctl", "start", serviceLabel)

	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("failed to start service: %w", err)
	}

	return nil
}

func stopService() error {
	cmd := exec.CommandContext(context.Background(), "launchctl", "stop", serviceLabel)

	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("failed to stop service: %w", err)
	}

	return nil
}

func restartService() error {
	_ = stopService()

	return startService()
}

func statusService() string {
	cmd := exec.CommandContext(context.Background(), "launchctl", "list", serviceLabel)

	_, err := cmd.Output()
	if err != nil {
		return "Service not loaded"
	}

	return "Service loaded"
}

func expandPath(path string) (string, error) {
	if strings.HasPrefix(path, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}

		return filepath.Join(home, path[1:]), nil
	}

	return path, nil
}
