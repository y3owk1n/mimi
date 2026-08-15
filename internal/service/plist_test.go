//nolint:testpackage // renderPlist and plistTemplate are unexported; the seam under test is intentionally internal.
package service

import (
	"strings"
	"testing"
)

// wantGoldenPlist is the full plist renderPlist must produce for binPath
// "/usr/local/bin/mimi", configPath "/Users/test/.config/mimi/config.toml" and
// logFile "/Users/test/.local/state/mimi/mimi.log" — written out by hand, not
// derived the way the code derives it, so this pins renderPlist to
// byte-identical output.
//
// It inherits from the pre-refactor cmd/mimi/cmd output, changed in exactly
// one place: issue #99 moved StandardOutPath/StandardErrorPath off the
// hardcoded /tmp/mimi.log and /tmp/mimi.err.log and onto log_file's directory.
const wantGoldenPlist = "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<!DOCTYPE plist PUBLIC \"-//Apple//DTD PLIST 1.0//EN\" \"http://www.apple.com/DTDs/PropertyList-1.0.dtd\">\n<plist version=\"1.0\">\n<dict>\n    <key>Label</key>\n    <string>com.y3owk1n.mimi</string>\n    <key>ProgramArguments</key>\n    <array>\n        <string>/usr/local/bin/mimi</string>\n        <string>start</string>\n        <string>--config</string>\n        <string>/Users/test/.config/mimi/config.toml</string>\n    </array>\n    <key>RunAtLoad</key>\n    <true/>\n    <key>KeepAlive</key>\n    <true/>\n    <key>StandardOutPath</key>\n    <string>/Users/test/.local/state/mimi/mimi.out.log</string>\n    <key>StandardErrorPath</key>\n    <string>/Users/test/.local/state/mimi/mimi.err.log</string>\n    <key>ProcessType</key>\n    <string>Interactive</string>\n    <key>LimitLoadToSessionType</key>\n    <string>Aqua</string>\n    <key>Nice</key>\n    <integer>-10</integer>\n    <key>ThrottleInterval</key>\n    <integer>10</integer>\n    <key>EnvironmentVariables</key>\n    <dict>\n        <key>PATH</key>\n        <string>/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin</string>\n    </dict>\n</dict>\n</plist>"

func TestRenderPlist_MatchesTheGoldenOutputByteForByte(t *testing.T) {
	got := renderPlist(
		"/usr/local/bin/mimi",
		"/Users/test/.config/mimi/config.toml",
		"/Users/test/.local/state/mimi/mimi.log",
	)
	if got != wantGoldenPlist {
		t.Errorf("renderPlist() =\n%q\nwant\n%q", got, wantGoldenPlist)
	}
}

// TestRenderPlist_CapturedStreamPaths pins where launchd is told to write the
// daemon's stdout and stderr for each shape settings.log_file can arrive in.
// Every expected value is written out literally rather than derived, and no
// case may name log_file itself: lumberjack rotates that file, so a second
// writer appending to it would corrupt the rotation.
func TestRenderPlist_CapturedStreamPaths(t *testing.T) {
	// The /tmp paths every fallback case expects — the ones the plist
	// hardcoded before issue #99 — spelled out here rather than read from the
	// package's own constants, so this test disagrees with the code if the
	// code changes them.
	const (
		wantFallbackStdout = "/tmp/mimi.log"
		wantFallbackStderr = "/tmp/mimi.err.log"
	)

	tests := []struct {
		name       string
		logFile    string
		wantStdout string
		wantStderr string
	}{
		{
			name:       "derived from log_file's directory with distinct names",
			logFile:    "/Users/test/.local/state/mimi/mimi.log",
			wantStdout: "/Users/test/.local/state/mimi/mimi.out.log",
			wantStderr: "/Users/test/.local/state/mimi/mimi.err.log",
		},
		{
			name:       "log_file with a non-default stem keeps that stem",
			logFile:    "/var/log/mimi/daemon.jsonl",
			wantStdout: "/var/log/mimi/daemon.out.log",
			wantStderr: "/var/log/mimi/daemon.err.log",
		},
		{
			name:       "log_file without an extension",
			logFile:    "/var/log/mimi-daemon",
			wantStdout: "/var/log/mimi-daemon.out.log",
			wantStderr: "/var/log/mimi-daemon.err.log",
		},
		{
			name:       "log_file unset falls back to /tmp",
			logFile:    "",
			wantStdout: wantFallbackStdout,
			wantStderr: wantFallbackStderr,
		},
		{
			// paths.ExpandHome returns the path unchanged when the home
			// directory cannot be resolved. launchd does not expand "~", so
			// such a value must not reach the plist.
			name:       "unexpanded ~ falls back to /tmp",
			logFile:    "~/.local/state/mimi/mimi.log",
			wantStdout: wantFallbackStdout,
			wantStderr: wantFallbackStderr,
		},
		{
			// launchd runs the job from /, so a relative path would land
			// somewhere the user never asked for.
			name:       "relative log_file falls back to /tmp",
			logFile:    "logs/mimi.log",
			wantStdout: wantFallbackStdout,
			wantStderr: wantFallbackStderr,
		},
		{
			name:       "log_file that is only an extension falls back to /tmp",
			logFile:    "/var/log/.log",
			wantStdout: wantFallbackStdout,
			wantStderr: wantFallbackStderr,
		},
		{
			name:       "log_file at the filesystem root falls back to /tmp",
			logFile:    "/",
			wantStdout: wantFallbackStdout,
			wantStderr: wantFallbackStderr,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			got := renderPlist(
				"/usr/local/bin/mimi",
				"/Users/test/.config/mimi/config.toml",
				testCase.logFile,
			)

			wantOut := "<key>StandardOutPath</key>\n    <string>" + testCase.wantStdout + "</string>"
			if !strings.Contains(got, wantOut) {
				t.Errorf(
					"renderPlist(logFile=%q) does not contain %q:\n%s",
					testCase.logFile,
					wantOut,
					got,
				)
			}

			wantErr := "<key>StandardErrorPath</key>\n    <string>" + testCase.wantStderr + "</string>"
			if !strings.Contains(got, wantErr) {
				t.Errorf(
					"renderPlist(logFile=%q) does not contain %q:\n%s",
					testCase.logFile,
					wantErr,
					got,
				)
			}

			if testCase.logFile != "" &&
				strings.Contains(got, "<string>"+testCase.logFile+"</string>") {
				t.Errorf(
					"renderPlist(logFile=%q) points a captured stream at log_file itself, which lumberjack rotates:\n%s",
					testCase.logFile,
					got,
				)
			}
		})
	}
}

func TestRenderPlist_Table(t *testing.T) {
	tests := []struct {
		name       string
		binPath    string
		configPath string
		logFile    string
	}{
		{
			name:       "typical paths",
			binPath:    "/opt/homebrew/bin/mimi",
			configPath: "~/.config/mimi/config.toml",
			logFile:    "/Users/test/.local/state/mimi/mimi.log",
		},
		{
			name:       "empty inputs",
			binPath:    "",
			configPath: "",
			logFile:    "",
		},
		{
			name:       "path containing the token-like substring MIMI",
			binPath:    "/Users/mimi-user/bin/mimi",
			configPath: "/Users/mimi-user/MIMI_CONFIG_PATH-lookalike/config.toml",
			logFile:    "/Users/mimi-user/MIMI_STDOUT_PATH-lookalike/mimi.log",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			got := renderPlist(testCase.binPath, testCase.configPath, testCase.logFile)

			wantBin := "<string>" + testCase.binPath + "</string>"
			if !strings.Contains(got, wantBin) {
				t.Errorf(
					"renderPlist(%q, %q) does not contain %q:\n%s",
					testCase.binPath,
					testCase.configPath,
					wantBin,
					got,
				)
			}

			wantConfig := "<string>" + testCase.configPath + "</string>"
			if !strings.Contains(got, wantConfig) {
				t.Errorf(
					"renderPlist(%q, %q) does not contain %q:\n%s",
					testCase.binPath,
					testCase.configPath,
					wantConfig,
					got,
				)
			}

			// The template's own literal Label must survive untouched.
			if !strings.Contains(got, "<string>com.y3owk1n.mimi</string>") {
				t.Errorf(
					"renderPlist(%q, %q) lost the Label string",
					testCase.binPath,
					testCase.configPath,
				)
			}
		})
	}
}
