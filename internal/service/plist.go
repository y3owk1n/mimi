package service

import "strings"

// Label is the launchd service identifier mimi installs itself under.
const Label = "com.y3owk1n.mimi"

// launchAgentsDir is where launchd expects a per-user agent's plist, in the
// form [paths.ExpandHome] accepts.
const launchAgentsDir = "~/Library/LaunchAgents"

// plistTemplate is the launchd plist mimi installs, with the binary and
// config paths left as tokens for renderPlist to fill in.
//
// StandardOutPath/StandardErrorPath are intentionally fixed at /tmp/mimi.log
// and /tmp/mimi.err.log rather than settings.log_file: that path only ever
// affects the captured stdout/stderr stream, and the file log sink honors
// settings.log_file independently. See issue #99.
const plistTemplate = `<?xml version="1.0" encoding="UTF-8"?>
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

// renderPlist fills plistTemplate with the binary and config paths mimi was
// given. It touches no filesystem and makes no system call: the same inputs
// always render the same bytes, which is what lets it be pinned with a plain
// table test instead of an integration one.
func renderPlist(binPath, configPath string) string {
	content := strings.ReplaceAll(plistTemplate, "MIMI_BINARY_PATH", binPath)

	return strings.ReplaceAll(content, "MIMI_CONFIG_PATH", configPath)
}
