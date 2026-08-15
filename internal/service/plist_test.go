//nolint:testpackage // renderPlist and plistTemplate are unexported; the seam under test is intentionally internal.
package service

import (
	"encoding/xml"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"
)

// The settings.log_file the tests in this package render against, and the
// captured stdout it puts beside that file. Both are written out here in full
// rather than derived, so a test using them still disagrees with the code if
// the code changes how one is placed relative to the other.
const (
	testLogFile        = "/Users/test/.local/state/mimi/mimi.log"
	testCapturedStdout = "/Users/test/.local/state/mimi/mimi.out.log"
)

// testBinPath is the installed binary the tests in this file render against.
// It is an input rather than an expectation: a case exercising some other
// value passes this one so that nothing but the value under test is unusual.
// testConfigPath, its counterpart for the config path, lives in
// service_test.go.
const testBinPath = "/usr/local/bin/mimi"

// wantGoldenPlist is the full plist renderPlist must produce for binPath
// "/usr/local/bin/mimi", configPath "/Users/test/.config/mimi/config.toml" and
// logFile "/Users/test/.local/state/mimi/mimi.log" — written out by hand, not
// derived the way the code derives it, so this pins renderPlist to
// byte-identical output.
//
// It inherits from the pre-refactor cmd/mimi/cmd output, changed in exactly
// two places: issue #99 moved StandardOutPath/StandardErrorPath off the
// hardcoded /tmp/mimi.log and /tmp/mimi.err.log and onto log_file's directory,
// and issue #157 added the two MIMI_CAPTURED_* environment entries the daemon
// reads those same paths back out of.
const wantGoldenPlist = "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<!DOCTYPE plist PUBLIC \"-//Apple//DTD PLIST 1.0//EN\" \"http://www.apple.com/DTDs/PropertyList-1.0.dtd\">\n<plist version=\"1.0\">\n<dict>\n    <key>Label</key>\n    <string>com.y3owk1n.mimi</string>\n    <key>ProgramArguments</key>\n    <array>\n        <string>/usr/local/bin/mimi</string>\n        <string>start</string>\n        <string>--config</string>\n        <string>/Users/test/.config/mimi/config.toml</string>\n    </array>\n    <key>RunAtLoad</key>\n    <true/>\n    <key>KeepAlive</key>\n    <true/>\n    <key>StandardOutPath</key>\n    <string>/Users/test/.local/state/mimi/mimi.out.log</string>\n    <key>StandardErrorPath</key>\n    <string>/Users/test/.local/state/mimi/mimi.err.log</string>\n    <key>ProcessType</key>\n    <string>Interactive</string>\n    <key>LimitLoadToSessionType</key>\n    <string>Aqua</string>\n    <key>Nice</key>\n    <integer>-10</integer>\n    <key>ThrottleInterval</key>\n    <integer>10</integer>\n    <key>EnvironmentVariables</key>\n    <dict>\n        <key>MIMI_CAPTURED_STDERR</key>\n        <string>/Users/test/.local/state/mimi/mimi.err.log</string>\n        <key>MIMI_CAPTURED_STDOUT</key>\n        <string>/Users/test/.local/state/mimi/mimi.out.log</string>\n        <key>PATH</key>\n        <string>/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin</string>\n    </dict>\n</dict>\n</plist>"

// TestRenderPlist_MatchesTheGoldenOutputByteForByte pins the plist rendered
// for a config that sets no service_path: the whole file, byte for byte,
// written out by hand.
//
// Every byte of it reaches an installed service, and only through `mimi
// services install` — so this is also what decides whether that command finds
// the plist on disk stale and replaces it. A change here is a change to every
// installed service on the next install, and has to be made deliberately.
func TestRenderPlist_MatchesTheGoldenOutputByteForByte(t *testing.T) {
	got := renderPlist(
		testBinPath,
		testConfigPath,
		testLogFile,
		"",
	)
	if got != wantGoldenPlist {
		t.Errorf("renderPlist() =\n%q\nwant\n%q", got, wantGoldenPlist)
	}
}

// TestRenderPlist_ServicePath pins the PATH the installed service runs with,
// which is the PATH its hooks inherit. Every expected value is written out
// literally: the point of the setting is that what the user typed is what the
// service gets.
func TestRenderPlist_ServicePath(t *testing.T) {
	tests := []struct {
		name        string
		servicePath string
		wantPath    string
	}{
		{
			name:        "unset keeps the PATH the plist has always hardcoded",
			servicePath: "",
			wantPath:    "/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin",
		},
		{
			name:        "a configured PATH replaces it entirely",
			servicePath: "/Users/test/.local/bin:/usr/bin:/bin",
			wantPath:    "/Users/test/.local/bin:/usr/bin:/bin",
		},
		{
			// A directory name may legally contain XML markup characters, and
			// a plist that is not well-formed XML is one launchd refuses to
			// load at login — a failure nothing in an install would report.
			name:        "XML markup in a directory name is escaped",
			servicePath: "/Users/test/bin & tools/<x>:/usr/bin",
			wantPath:    "/Users/test/bin &amp; tools/&lt;x&gt;:/usr/bin",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			got := renderPlist(
				testBinPath,
				testConfigPath,
				testLogFile,
				testCase.servicePath,
			)

			want := "<key>PATH</key>\n        <string>" + testCase.wantPath + "</string>"
			if !strings.Contains(got, want) {
				t.Errorf(
					"renderPlist(servicePath=%q) does not contain %q:\n%s",
					testCase.servicePath,
					want,
					got,
				)
			}
		})
	}
}

// TestRenderPlist_CapturedStreamPaths pins where launchd is told to write the
// daemon's stdout and stderr for each shape settings.log_file can arrive in,
// and that the environment entries the daemon reads those paths back out of
// name the same two files. Every expected value is written out literally rather
// than derived, and no case may name log_file itself: lumberjack rotates that
// file, so a second writer appending to it would corrupt the rotation.
//
// The environment is the whole of what tells a daemon those files exist and are
// its to empty at startup — one started by hand has a terminal on stdout, sees
// neither entry, and truncates nothing. Its names are spelled literally here,
// because internal/daemon spells the same two literals and nothing else holds
// the two sides together: a rename has to fail a test on the side that made it.
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
			logFile:    testLogFile,
			wantStdout: testCapturedStdout,
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
				testBinPath,
				testConfigPath,
				testCase.logFile,
				"",
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

			environment := map[string]string{
				"MIMI_CAPTURED_STDOUT": testCase.wantStdout,
				"MIMI_CAPTURED_STDERR": testCase.wantStderr,
			}
			for key, want := range environment {
				wantEntry := "<key>" + key + "</key>\n        <string>" + want + "</string>"
				if !strings.Contains(got, wantEntry) {
					t.Errorf(
						"renderPlist(logFile=%q) does not contain %q:\n%s",
						testCase.logFile,
						wantEntry,
						got,
					)
				}
			}
		})
	}
}

// TestRenderPlist_EscapesEveryValueItSubstitutes pins that a path carrying an
// XML metacharacter reaches the plist escaped, whichever of the substituted
// values carries it. A directory may legally be named with "&", "<" or ">", and
// a plist that is not well-formed XML is one launchd refuses to load at login —
// long after the install that wrote it reported success.
//
// Each case asserts both halves of that: the exact escaped bytes, so the
// escaping cannot quietly change, and the value read back out of a real XML
// parse, so escaping that produced something a parser resolves to a different
// path would fail here rather than at login.
func TestRenderPlist_EscapesEveryValueItSubstitutes(t *testing.T) {
	// The PATH the service-path case configures, named because the case both
	// passes it in and expects it back out of the parse — the whole point being
	// that those are the same string.
	const markupServicePath = "/Users/test/bin & tools/<x>:/usr/bin"

	tests := []struct {
		name        string
		binPath     string
		configPath  string
		logFile     string
		servicePath string
		// wantEscaped are the rendered fragments, spelled out with the
		// escapes the plist must carry.
		wantEscaped []string
		// wantDecoded are the values an XML parser must hand back, spelled
		// out raw — what launchd ends up executing, opening or exporting.
		wantDecoded []string
	}{
		{
			name:        "binary path",
			binPath:     "/Users/test/bin & <tools>/mimi",
			configPath:  testConfigPath,
			logFile:     testLogFile,
			servicePath: "",
			wantEscaped: []string{"<string>/Users/test/bin &amp; &lt;tools&gt;/mimi</string>"},
			wantDecoded: []string{"/Users/test/bin & <tools>/mimi"},
		},
		{
			name:        "config path",
			binPath:     testBinPath,
			configPath:  "/Users/test/.config/mimi & <x>/config.toml",
			logFile:     testLogFile,
			servicePath: "",
			wantEscaped: []string{
				"<string>/Users/test/.config/mimi &amp; &lt;x&gt;/config.toml</string>",
			},
			wantDecoded: []string{"/Users/test/.config/mimi & <x>/config.toml"},
		},
		{
			// Both captured-stream paths are derived from log_file, and each is
			// substituted twice: once for launchd to write to, once for the
			// environment entry the daemon reads it back out of.
			name:        "captured stream paths derived from log_file",
			binPath:     testBinPath,
			configPath:  testConfigPath,
			logFile:     "/Users/test/logs & <x>/mimi.log",
			servicePath: "",
			wantEscaped: []string{
				"<key>StandardOutPath</key>\n    <string>/Users/test/logs &amp; &lt;x&gt;/mimi.out.log</string>",
				"<key>StandardErrorPath</key>\n    <string>/Users/test/logs &amp; &lt;x&gt;/mimi.err.log</string>",
				"<key>MIMI_CAPTURED_STDOUT</key>\n        <string>/Users/test/logs &amp; &lt;x&gt;/mimi.out.log</string>",
				"<key>MIMI_CAPTURED_STDERR</key>\n        <string>/Users/test/logs &amp; &lt;x&gt;/mimi.err.log</string>",
			},
			wantDecoded: []string{
				"/Users/test/logs & <x>/mimi.out.log",
				"/Users/test/logs & <x>/mimi.err.log",
			},
		},
		{
			name:        "service path",
			binPath:     testBinPath,
			configPath:  testConfigPath,
			logFile:     testLogFile,
			servicePath: markupServicePath,
			wantEscaped: []string{
				"<key>PATH</key>\n        <string>/Users/test/bin &amp; tools/&lt;x&gt;:/usr/bin</string>",
			},
			wantDecoded: []string{markupServicePath},
		},
		{
			// A carriage return is legal in a macOS path and well-formed in an
			// XML text node, but every parser rewrites a raw one to a newline —
			// so launchd would be handed a path the user never configured. It
			// has to reach the plist as a character reference.
			name:        "path carrying a carriage return an XML parser would rewrite",
			binPath:     testBinPath,
			configPath:  "/Users/test/.config/mi\rmi/config.toml",
			logFile:     testLogFile,
			servicePath: "",
			wantEscaped: []string{"<string>/Users/test/.config/mi&#xD;mi/config.toml</string>"},
			wantDecoded: []string{"/Users/test/.config/mi\rmi/config.toml"},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			got := renderPlist(
				testCase.binPath,
				testCase.configPath,
				testCase.logFile,
				testCase.servicePath,
			)

			for _, want := range testCase.wantEscaped {
				if !strings.Contains(got, want) {
					t.Errorf("renderPlist() does not contain %q:\n%s", want, got)
				}
			}

			decoded := parsePlistStrings(t, got)
			for _, want := range testCase.wantDecoded {
				if !slices.Contains(decoded, want) {
					t.Errorf("parsed plist has no <string> %q, only %q", want, decoded)
				}
			}
		})
	}
}

// TestRenderPlist_SubstitutedValueNamedAfterAnotherToken pins that a path which
// happens to be named after one of the template's *other* tokens reaches the
// plist literally. Issue #177 demonstrated it with a config path of
// "/Users/test/MIMI_SERVICE_PATH/config.toml", which rendered as
// "/Users/test//opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin/config.toml" —
// the service PATH spliced into the middle of the user's path, so the plist
// pointed launchd at a file that does not exist.
//
// This is not the case TestRenderPlist_Table's "path containing the token-like
// substring MIMI" covers. There each value carries only its *own* token name,
// which survives however the substitution is arranged: by the time such a value
// is in place, whatever would have matched it has already run. The names that
// break are the ones belonging to a *later* value, and only a substitution that
// rescans what it has already inserted can break them. Neither test subsumes
// the other, and the cross-value one is the one that failed before #177.
//
// Every case is checked against every token name, including its own, so that
// the property under test is the whole of it: no substituted value is ever
// rescanned, whichever token name it happens to contain.
func TestRenderPlist_SubstitutedValueNamedAfterAnotherToken(t *testing.T) {
	// Every token renderPlist fills in, spelled out here rather than read from
	// the template, so a token this test has stopped covering is a token the
	// code has to have renamed.
	tokens := []string{
		"MIMI_BINARY_PATH",
		"MIMI_CONFIG_PATH",
		"MIMI_STDOUT_PATH",
		"MIMI_STDERR_PATH",
		"MIMI_SERVICE_PATH",
	}

	tests := []struct {
		name string
		// render renders a plist in which the one input this case is about
		// names dir as a directory component and every other input is ordinary,
		// and returns it alongside the values that input must produce verbatim.
		render func(dir string) (plist string, wantValues []string)
	}{
		{
			name: "the binary path",
			render: func(dir string) (string, []string) {
				binPath := "/Users/test/" + dir + "/mimi"

				return renderPlist(binPath, testConfigPath, testLogFile, ""), []string{binPath}
			},
		},
		{
			name: "the config path",
			render: func(dir string) (string, []string) {
				configPath := "/Users/test/" + dir + "/config.toml"

				return renderPlist(testBinPath, configPath, testLogFile, ""), []string{configPath}
			},
		},
		{
			// log_file yields two values, substituted for two different tokens,
			// so each of them also has to survive the other's name.
			name: "the captured stream paths derived from log_file",
			render: func(dir string) (string, []string) {
				logFile := "/Users/test/" + dir + "/mimi.log"

				return renderPlist(testBinPath, testConfigPath, logFile, ""), []string{
					"/Users/test/" + dir + "/mimi.out.log",
					"/Users/test/" + dir + "/mimi.err.log",
				}
			},
		},
		{
			name: "the service PATH",
			render: func(dir string) (string, []string) {
				servicePath := "/Users/test/" + dir + "/bin:/usr/bin"

				return renderPlist(
					testBinPath,
					testConfigPath,
					testLogFile,
					servicePath,
				), []string{servicePath}
			},
		},
	}

	for _, testCase := range tests {
		for _, token := range tokens {
			t.Run(testCase.name+" named after "+token, func(t *testing.T) {
				got, wantValues := testCase.render(token)

				for _, wantValue := range wantValues {
					want := "<string>" + wantValue + "</string>"
					if !strings.Contains(got, want) {
						t.Errorf("renderPlist() does not contain %q:\n%s", want, got)
					}
				}
			})
		}
	}
}

// parsePlistStrings decodes a rendered plist with a real XML parser and returns
// every <string> value it holds. Failing to parse fails the test: that is the
// whole of what launchd rejects a plist for, and the only check that a value
// this package escaped by hand is a value a parser agrees with.
func parsePlistStrings(t *testing.T, content string) []string {
	t.Helper()

	var (
		decoder  = xml.NewDecoder(strings.NewReader(content))
		values   []string
		inString bool
	)

	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			t.Fatalf("rendered plist is not well-formed XML: %v\n%s", err, content)
		}

		switch typed := token.(type) {
		case xml.StartElement:
			inString = typed.Name.Local == "string"
			if inString {
				values = append(values, "")
			}
		case xml.CharData:
			if inString {
				values[len(values)-1] += string(typed)
			}
		case xml.EndElement:
			inString = false
		}
	}

	return values
}

func TestRenderPlist_Table(t *testing.T) {
	tests := []struct {
		name        string
		binPath     string
		configPath  string
		logFile     string
		servicePath string
	}{
		{
			name:        "typical paths",
			binPath:     "/opt/homebrew/bin/mimi",
			configPath:  "~/.config/mimi/config.toml",
			logFile:     testLogFile,
			servicePath: "/usr/bin:/bin",
		},
		{
			name:        "empty inputs",
			binPath:     "",
			configPath:  "",
			logFile:     "",
			servicePath: "",
		},
		{
			name:        "path containing the token-like substring MIMI",
			binPath:     "/Users/mimi-user/bin/mimi",
			configPath:  "/Users/mimi-user/MIMI_CONFIG_PATH-lookalike/config.toml",
			logFile:     "/Users/mimi-user/MIMI_STDOUT_PATH-lookalike/mimi.log",
			servicePath: "/Users/mimi-user/MIMI_SERVICE_PATH-lookalike/bin:/usr/bin",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			got := renderPlist(
				testCase.binPath,
				testCase.configPath,
				testCase.logFile,
				testCase.servicePath,
			)

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
