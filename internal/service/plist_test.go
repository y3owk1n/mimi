//nolint:testpackage // renderPlist and plistTemplate are unexported; the seam under test is intentionally internal.
package service

import (
	"strings"
	"testing"
)

// wantGoldenPlist is what the pre-refactor cmd/mimi/cmd implementation
// produced for binPath "/usr/local/bin/mimi" and configPath
// "/Users/test/.config/mimi/config.toml" — captured from that code before it
// moved, so this pins renderPlist to byte-identical output rather than to a
// value this test derives the same way the code does.
const wantGoldenPlist = "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<!DOCTYPE plist PUBLIC \"-//Apple//DTD PLIST 1.0//EN\" \"http://www.apple.com/DTDs/PropertyList-1.0.dtd\">\n<plist version=\"1.0\">\n<dict>\n    <key>Label</key>\n    <string>com.y3owk1n.mimi</string>\n    <key>ProgramArguments</key>\n    <array>\n        <string>/usr/local/bin/mimi</string>\n        <string>start</string>\n        <string>--config</string>\n        <string>/Users/test/.config/mimi/config.toml</string>\n    </array>\n    <key>RunAtLoad</key>\n    <true/>\n    <key>KeepAlive</key>\n    <true/>\n    <key>StandardOutPath</key>\n    <string>/tmp/mimi.log</string>\n    <key>StandardErrorPath</key>\n    <string>/tmp/mimi.err.log</string>\n    <key>ProcessType</key>\n    <string>Interactive</string>\n    <key>LimitLoadToSessionType</key>\n    <string>Aqua</string>\n    <key>Nice</key>\n    <integer>-10</integer>\n    <key>ThrottleInterval</key>\n    <integer>10</integer>\n    <key>EnvironmentVariables</key>\n    <dict>\n        <key>PATH</key>\n        <string>/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin</string>\n    </dict>\n</dict>\n</plist>"

func TestRenderPlist_MatchesThePreRefactorOutputByteForByte(t *testing.T) {
	got := renderPlist("/usr/local/bin/mimi", "/Users/test/.config/mimi/config.toml")
	if got != wantGoldenPlist {
		t.Errorf("renderPlist() =\n%q\nwant\n%q", got, wantGoldenPlist)
	}
}

func TestRenderPlist_Table(t *testing.T) {
	tests := []struct {
		name       string
		binPath    string
		configPath string
	}{
		{
			name:       "typical paths",
			binPath:    "/opt/homebrew/bin/mimi",
			configPath: "~/.config/mimi/config.toml",
		},
		{
			name:       "empty inputs",
			binPath:    "",
			configPath: "",
		},
		{
			name:       "path containing the token-like substring MIMI",
			binPath:    "/Users/mimi-user/bin/mimi",
			configPath: "/Users/mimi-user/MIMI_CONFIG_PATH-lookalike/config.toml",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			got := renderPlist(testCase.binPath, testCase.configPath)

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
