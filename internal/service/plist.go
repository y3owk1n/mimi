package service

import (
	"path/filepath"
	"strings"
)

// Label is the launchd service identifier mimi installs itself under.
const Label = "com.y3owk1n.mimi"

// launchAgentsDir is where launchd expects a per-user agent's plist, in the
// form [paths.ExpandHome] accepts.
const launchAgentsDir = "~/Library/LaunchAgents"

// Default paths for the captured stdout/stderr streams, used when
// settings.log_file gives nothing usable to derive them from. These are the
// paths the plist has always hardcoded, kept unchanged so the fallback moves
// nobody's files and stays identical to what the Nix modules install. /tmp is
// also the one directory guaranteed to exist and be writable before launchd
// spawns the job. See issue #99.
const (
	defaultStdoutPath = "/tmp/mimi.log"
	defaultStderrPath = "/tmp/mimi.err.log"
)

// plistTemplate is the launchd plist mimi installs, with the binary, config
// and captured-stream paths left as tokens for renderPlist to fill in.
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
    <string>MIMI_STDOUT_PATH</string>
    <key>StandardErrorPath</key>
    <string>MIMI_STDERR_PATH</string>
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
// given, plus the stdout/stderr capture paths derived from logFile. It touches
// no filesystem and makes no system call: the same inputs always render the
// same bytes, which is what lets it be pinned with a plain table test instead
// of an integration one.
func renderPlist(binPath, configPath, logFile string) string {
	streams := capturedStreamsFor(logFile)

	content := strings.ReplaceAll(plistTemplate, "MIMI_BINARY_PATH", binPath)
	content = strings.ReplaceAll(content, "MIMI_CONFIG_PATH", configPath)
	content = strings.ReplaceAll(content, "MIMI_STDOUT_PATH", streams.stdout)

	return strings.ReplaceAll(content, "MIMI_STDERR_PATH", streams.stderr)
}

// capturedStreams is where launchd writes the daemon's stdout and stderr. The
// two always share a directory, so callers that need it can take it from
// either.
type capturedStreams struct {
	stdout string
	stderr string
}

// dir is the directory both captured streams live in.
func (c capturedStreams) dir() string {
	return filepath.Dir(c.stdout)
}

// capturedStreamsFor derives where launchd writes the daemon's stdout and
// stderr from settings.log_file: same directory, same stem, distinct
// ".out.log" and ".err.log" suffixes. Neither can ever equal logFile itself —
// that file belongs to lumberjack, which rotates it, and a second writer
// appending raw console output to it would corrupt the rotation.
//
// logFile is optional and reaches here exactly as config stored it, so it may
// be empty or — when paths.ExpandHome could not resolve the home directory —
// still carry a literal "~". launchd expands neither "~" nor a relative path,
// so anything that is not already absolute falls back to /tmp rather than
// putting a path launchd cannot open into the plist.
func capturedStreamsFor(logFile string) capturedStreams {
	fallback := capturedStreams{stdout: defaultStdoutPath, stderr: defaultStderrPath}

	if !filepath.IsAbs(logFile) {
		return fallback
	}

	dir, base := filepath.Dir(logFile), filepath.Base(logFile)

	stem := strings.TrimSuffix(base, filepath.Ext(base))
	if stem == "" || stem == "." || stem == string(filepath.Separator) {
		return fallback
	}

	return capturedStreams{
		stdout: filepath.Join(dir, stem+".out.log"),
		stderr: filepath.Join(dir, stem+".err.log"),
	}
}
