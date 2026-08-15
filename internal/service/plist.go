package service

import (
	"encoding/xml"
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
//
// Each captured-stream token appears twice: once as the path launchd writes
// that stream to, and once as the MIMI_CAPTURED_STDOUT / MIMI_CAPTURED_STDERR
// environment entry the daemon reads it back out of. One substitution fills
// both, so the two can never name different files.
//
// Those entries are the whole of what tells a daemon that these files exist
// and are its to truncate once at startup. A daemon started any other way —
// `mimi start` by hand, with a terminal on stdout — sees neither and truncates
// nothing. internal/daemon spells the same two names and deliberately does not
// import this package to learn them: this is the install-time surface, and the
// daemon is not its consumer. The golden plist test and the daemon's own test
// each spell the literals, so a rename on either side fails a test there.
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
        <key>MIMI_CAPTURED_STDERR</key>
        <string>MIMI_STDERR_PATH</string>
        <key>MIMI_CAPTURED_STDOUT</key>
        <string>MIMI_STDOUT_PATH</string>
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
//
// Every value it substitutes goes through escapeXMLText, without exception:
// each one is a path the user chose, and any of them may legally be named with
// something that is markup here. Escaping changes nothing about an ordinary
// path, which is why the golden test still pins the same bytes.
func renderPlist(binPath, configPath, logFile, servicePath string) string {
	streams := capturedStreamsFor(logFile)

	content := strings.ReplaceAll(plistTemplate, "MIMI_BINARY_PATH", escapeXMLText(binPath))
	content = strings.ReplaceAll(content, "MIMI_CONFIG_PATH", escapeXMLText(configPath))
	content = strings.ReplaceAll(content, "MIMI_STDOUT_PATH", escapeXMLText(streams.stdout))
	content = strings.ReplaceAll(content, "MIMI_STDERR_PATH", escapeXMLText(streams.stderr))

	return strings.ReplaceAll(
		content,
		"MIMI_SERVICE_PATH",
		escapeXMLText(servicePathFor(servicePath)),
	)
}

// escapeXMLText renders a value as the XML text node it is substituted into, so
// that a value carrying markup cannot break out of the <string> around it. A
// plist that is not well-formed is one launchd silently refuses at login, which
// is the failure mode furthest from where it would be noticed — the install
// that wrote it has already reported success.
//
// It is encoding/xml's own escaper rather than a replacer over "&", "<" and
// ">", because the set of characters a path must not carry into XML raw is
// larger than those three and is not obvious from reading the file. A carriage
// return is the one that matters: it is legal in a macOS path, well-formed in a
// text node, and rewritten to a newline by every conformant parser — so a raw
// one would hand launchd a different path than the user configured, silently.
// escapeXMLText writes it as &#xD;, along with the tab and newline that have
// the same problem in weaker form.
//
// The price is that it also escapes the quote and apostrophe an XML text node
// does not require escaped. Those render as &#34; and &#39;, which every parser
// resolves back to the character itself; a path carrying one is rendered more
// verbosely than it strictly needs, never wrongly.
//
// A path that is not valid UTF-8 has no XML representation at all, and
// encoding/xml substitutes U+FFFD. Such a path cannot reach launchd intact by
// any escaping; what this buys is a plist launchd still parses, failing at a
// path that does not exist rather than at a file it will not read.
func escapeXMLText(value string) string {
	var escaped strings.Builder

	// EscapeText fails only when its writer does, and a strings.Builder never
	// does.
	_ = xml.EscapeText(&escaped, []byte(value))

	return escaped.String()
}

// servicePathFor is the PATH value the plist gets for a given
// settings.service_path: the configured one or, when the setting is unset, the
// default. Escaping is renderPlist's, applied to every substituted value alike.
func servicePathFor(servicePath string) string {
	if servicePath == "" {
		return defaultServicePath
	}

	return servicePath
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
