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

// defaultServicePath is the PATH the installed service runs with when
// settings.service_path says nothing. It is the value the plist hardcoded
// before that setting existed, kept exactly so that a config which does not
// mention it renders the same bytes as ever and leaves installed services
// alone.
const defaultServicePath = "/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin"

// plistTemplate is the launchd plist mimi installs, with the binary, config,
// captured-stream and PATH values left as tokens for renderPlist to fill in.
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
        <string>MIMI_SERVICE_PATH</string>
    </dict>
</dict>
</plist>`

// renderPlist fills plistTemplate with the binary and config paths mimi was
// given, the stdout/stderr capture paths derived from logFile, and the PATH the
// service runs with. It touches no filesystem and makes no system call: the
// same inputs always render the same bytes, which is what lets it be pinned
// with a plain table test instead of an integration one.
//
// servicePath is settings.service_path, and is optional: an empty one renders
// the PATH the plist hardcoded before that setting existed, so a config that
// never mentions it produces the same bytes as ever.
func renderPlist(binPath, configPath, logFile, servicePath string) string {
	streams := capturedStreamsFor(logFile)

	content := strings.ReplaceAll(plistTemplate, "MIMI_BINARY_PATH", binPath)
	content = strings.ReplaceAll(content, "MIMI_CONFIG_PATH", configPath)
	content = strings.ReplaceAll(content, "MIMI_STDOUT_PATH", streams.stdout)
	content = strings.ReplaceAll(content, "MIMI_STDERR_PATH", streams.stderr)

	return strings.ReplaceAll(content, "MIMI_SERVICE_PATH", servicePathFor(servicePath))
}

// xmlTextEscaper escapes the characters that end an XML text node, so a value
// carrying one cannot break out of the <string> it is rendered into. A plist
// that is not well-formed is one launchd silently refuses at login, which is
// the failure mode furthest from where it would be noticed.
//
// It is applied to service_path only, because that is the value this render
// gained. The binary, config and captured-stream paths have always been
// substituted raw and still are; a directory named with an XML metacharacter
// breaks the plist there too, and fixing that is its own change rather than a
// side effect of adding a setting.
//
//nolint:gochecknoglobals // an immutable replacer, built once
var xmlTextEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")

// servicePathFor is the PATH value the plist gets for a given
// settings.service_path: the configured one, escaped for XML, or the default
// when the setting is unset.
func servicePathFor(servicePath string) string {
	if servicePath == "" {
		return defaultServicePath
	}

	return xmlTextEscaper.Replace(servicePath)
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
