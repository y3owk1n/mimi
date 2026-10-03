//nolint:testpackage
package logging

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/y3owk1n/mimi/internal/config"
	"github.com/y3owk1n/mimi/internal/events"
)

// newTestConfig is a config with nothing set but the logging settings a test
// cares about.
func newTestConfig(format, logFile string) *config.Config {
	cfg := &config.Config{}
	cfg.Settings.LogLevel = "debug"
	cfg.Settings.LogFormat = format
	cfg.Settings.LogFile = logFile

	return cfg
}

// newTestLogger builds a logger whose console sink is an in-memory buffer, so
// the encoder selection can be asserted without a real terminal.
func newTestLogger(format string, isTerminal bool) (*zap.SugaredLogger, *bytes.Buffer) {
	buf := &bytes.Buffer{}

	return newLogger(newTestConfig(format, ""), zapcore.AddSync(buf), isTerminal), buf
}

func TestNewLogger_JSONFormatWritesJSONToConsole(t *testing.T) {
	logger, buf := newTestLogger(formatJSON, false)

	logger.Infow("hello", "count", 1)

	var entry map[string]any

	err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &entry)
	if err != nil {
		t.Fatalf("console output is not JSON: %v (output %q)", err, buf.String())
	}

	if entry["msg"] != "hello" {
		t.Errorf("msg = %v, want %q", entry["msg"], "hello")
	}

	if entry["count"] != float64(1) {
		t.Errorf("count = %v, want 1", entry["count"])
	}
}

func TestNewLogger_TextFormatWritesOneLinePerEntry(t *testing.T) {
	const timestamp = `^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}\.\d{3}(Z|[+-]\d{2}:\d{2}) `

	tests := []struct {
		name string
		log  func(*zap.SugaredLogger)
		want string
	}{
		{
			name: "named logger with context and entry fields in order",
			log: func(logger *zap.SugaredLogger) {
				logger.Named("tiling").With("layout", "bsp").
					Infow("pass applied", "windows", 3, "focused", true)
			},
			want: ` INFO tiling: pass applied layout=bsp windows=3 focused=true$`,
		},
		{
			name: "unnamed logger targets the calling file and quotes spaced values",
			log:  func(logger *zap.SugaredLogger) { logger.Warnw("option inert", "option", "a b") },
			want: ` WARN logging/logger_test\.go:\d+: option inert option="a b"$`,
		},
		{
			name: "named string type prints bare",
			log: func(logger *zap.SugaredLogger) {
				logger.Named("observe").Debugw("event", "kind", events.EventKind("window_created"))
			},
			want: `DEBUG observe: event kind=window_created$`,
		},
		{
			name: "newline in message stays on one line",
			log:  func(logger *zap.SugaredLogger) { logger.Named("hooks").Info("first\nsecond") },
			want: ` INFO hooks: first\\nsecond$`,
		},
	}

	for _, format := range []string{formatText, ""} {
		for _, testCase := range tests {
			t.Run("format "+format+"/"+testCase.name, func(t *testing.T) {
				logger, buf := newTestLogger(format, false)

				testCase.log(logger)

				output := strings.TrimSuffix(buf.String(), "\n")
				if strings.Contains(output, "\n") {
					t.Fatalf("console output spans more than one line: %q", output)
				}

				if strings.Contains(output, "\x1b[") {
					t.Errorf("non-terminal console output carries color escapes: %q", output)
				}

				pattern := regexp.MustCompile(timestamp + testCase.want)
				if !pattern.MatchString(output) {
					t.Errorf("console line = %q, want it to match %q", output, pattern)
				}
			})
		}
	}
}

func TestNewLogger_TextFormatColorsTheLevelOnATerminal(t *testing.T) {
	logger, buf := newTestLogger(formatText, true)

	logger.Info("hello")

	if !strings.Contains(buf.String(), "\x1b[") {
		t.Errorf("terminal console output lost its color escapes: %q", buf.String())
	}
}

func TestNewLogger_JSONFormatCarriesNoColorOnATerminal(t *testing.T) {
	logger, buf := newTestLogger(formatJSON, true)

	logger.Infow("hello")

	if strings.Contains(buf.String(), "\x1b[") {
		t.Errorf("json console output carries color escapes: %q", buf.String())
	}
}

func TestNewLogger_UnknownFormatWarnsAndFallsBackToText(t *testing.T) {
	logger, buf := newTestLogger("yaml", false)

	warning := buf.String()
	if !strings.Contains(warning, "WARN") || !strings.Contains(warning, "settings.log_format") {
		t.Errorf("an unknown log_format was accepted silently: %q", warning)
	}

	buf.Reset()
	logger.Info("hello")

	if !strings.Contains(buf.String(), " hello") {
		t.Errorf("unknown log_format did not fall back to text: %q", buf.String())
	}
}

func TestNewLogger_KnownFormatWarnsAboutNothing(t *testing.T) {
	for _, format := range []string{formatText, "TEXT", formatJSON, "JSON", ""} {
		t.Run("format "+format, func(t *testing.T) {
			_, buf := newTestLogger(format, false)

			if buf.Len() != 0 {
				t.Errorf("a known log_format warned: %q", buf.String())
			}
		})
	}
}

func TestNewLogger_LogFileStaysJSONForEveryFormat(t *testing.T) {
	for _, format := range []string{formatText, formatJSON} {
		t.Run("format "+format, func(t *testing.T) {
			logPath := filepath.Join(t.TempDir(), "mimi.log")

			cfg := newTestConfig(format, logPath)

			logger := newLogger(cfg, zapcore.AddSync(&bytes.Buffer{}), true)

			logger.Infow("hello", "count", 1)

			err := logger.Sync()
			if err != nil {
				t.Fatalf("sync: %v", err)
			}

			written, err := os.ReadFile(logPath)
			if err != nil {
				t.Fatalf("read log file: %v", err)
			}

			var entry map[string]any

			err = json.Unmarshal(bytes.TrimSpace(written), &entry)
			if err != nil {
				t.Fatalf("log file is not JSON: %v (contents %q)", err, written)
			}

			if entry["msg"] != "hello" {
				t.Errorf("msg = %v, want %q", entry["msg"], "hello")
			}

			if strings.Contains(string(written), "\x1b[") {
				t.Errorf("log file carries color escapes: %q", written)
			}
		})
	}
}

func TestNewLogger_UnknownLevelWarnsAndFallsBackToInfo(t *testing.T) {
	cfg := newTestConfig(formatText, "")
	cfg.Settings.LogLevel = "verbose"

	buf := &bytes.Buffer{}
	logger := newLogger(cfg, zapcore.AddSync(buf), false)

	warning := buf.String()
	if !strings.Contains(warning, "WARN") || !strings.Contains(warning, "settings.log_level") {
		t.Errorf("an unknown log_level was accepted silently: %q", warning)
	}

	if strings.Contains(warning, "verbose") {
		t.Errorf("the warning logged the user's config text: %q", warning)
	}

	buf.Reset()
	logger.Debug("hidden")
	logger.Info("shown")

	if strings.Contains(buf.String(), "hidden") || !strings.Contains(buf.String(), "shown") {
		t.Errorf("unknown log_level did not fall back to info: %q", buf.String())
	}
}
