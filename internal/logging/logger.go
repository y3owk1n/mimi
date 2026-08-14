package logging

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"golang.org/x/term"
	"gopkg.in/natefinch/lumberjack.v2"

	"github.com/y3owk1n/mimi/internal/config"
	"github.com/y3owk1n/mimi/internal/events"
	"github.com/y3owk1n/mimi/internal/paths"
)

const (
	logMaxSizeMB  = 100
	logMaxBackups = 3
	logMaxAgeDays = 28
)

// Recognized values of the settings.log_format setting. It selects the console
// encoder only; the log file is always JSON.
const (
	formatText = "text"
	formatJSON = "json"
)

// New creates a zap sugared logger with console and optional file output.
func New(cfg *config.Config) *zap.SugaredLogger {
	consoleWriter := os.Stdout

	return newLogger(cfg, zapcore.AddSync(consoleWriter), term.IsTerminal(int(consoleWriter.Fd())))
}

// newLogger builds the logger against an explicit console sink, so tests can
// choose both the sink and whether it counts as a terminal.
func newLogger(
	cfg *config.Config,
	consoleWriter zapcore.WriteSyncer,
	isTerminal bool,
) *zap.SugaredLogger {
	level := parseLevel(cfg.Settings.LogLevel)
	format, knownFormat := parseFormat(cfg.Settings.LogFormat)

	cores := []zapcore.Core{
		zapcore.NewCore(consoleEncoder(format, isTerminal), consoleWriter, level),
	}

	if cfg.Settings.LogFile != "" {
		logWriter := &lumberjack.Logger{
			Filename:   paths.ExpandHome(cfg.Settings.LogFile),
			MaxSize:    logMaxSizeMB,
			MaxBackups: logMaxBackups,
			MaxAge:     logMaxAgeDays,
		}

		// The file log is JSON for machine parsing, whatever log_format says.
		fileEncoder := zapcore.NewJSONEncoder(jsonEncoderConfig())

		cores = append(cores, zapcore.NewCore(fileEncoder, zapcore.AddSync(logWriter), level))
	}

	core := zapcore.NewTee(cores...)

	logger := zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel)).Sugar()

	if !knownFormat {
		// The value itself stays out of the log; it is the user's config text.
		logger.Warnw(
			"unrecognized settings.log_format, using text",
			"valid",
			formatText+"|"+formatJSON,
		)
	}

	return logger
}

// parseFormat maps a configured log_format onto the console format it selects,
// reporting whether the value was recognized. An empty value is the unset
// default, and anything unrecognized falls back to text.
func parseFormat(format string) (string, bool) {
	switch {
	case format == "", strings.EqualFold(format, formatText):
		return formatText, true
	case strings.EqualFold(format, formatJSON):
		return formatJSON, true
	default:
		return formatText, false
	}
}

// consoleEncoder builds the console encoder for a parsed log format:
// human-readable for text, JSON for json. Only the human-readable encoder
// colorizes its level, and only when the console is a terminal.
func consoleEncoder(format string, isTerminal bool) zapcore.Encoder {
	if format == formatJSON {
		return zapcore.NewJSONEncoder(jsonEncoderConfig())
	}

	encoderConfig := zap.NewDevelopmentEncoderConfig()

	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	if isTerminal {
		encoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	} else {
		encoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder
	}

	return zapcore.NewConsoleEncoder(encoderConfig)
}

// jsonEncoderConfig is the machine-parseable encoder configuration, shared by
// the log file and by the console when log_format is "json".
func jsonEncoderConfig() zapcore.EncoderConfig {
	encoderConfig := zap.NewProductionEncoderConfig()

	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	encoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder

	return encoderConfig
}

// WriteEventLog subscribes to the event bus and writes JSON events to a log file.
func WriteEventLog(
	ctx context.Context,
	sub events.Subscriber,
	logPath string,
	logger *zap.SugaredLogger,
) {
	if logPath == "" {
		return
	}

	eventLogPath := logPath + ".events.jsonl"

	logFile, err := openAppend(eventLogPath)
	if err != nil {
		logger.Warnw("cannot open event log", "err", err)

		return
	}

	defer func() { _ = logFile.Close() }()

	enc := json.NewEncoder(logFile)
	for {
		select {
		case <-ctx.Done():
			return
		case e, ok := <-sub:
			if !ok {
				return
			}

			err := enc.Encode(e)
			if err != nil {
				logger.Warnw("event log write error", "err", err)
			}
		}
	}
}

func openAppend(path string) (*os.File, error) {
	path = paths.ExpandHome(path)

	err := os.MkdirAll(filepath.Dir(path), 0o755) //nolint:mnd
	if err != nil {
		return nil, err
	}

	return os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644) //nolint:mnd
}

func parseLevel(s string) zapcore.Level {
	switch strings.ToLower(s) {
	case "debug":
		return zapcore.DebugLevel
	case "info":
		return zapcore.InfoLevel
	case "warn", "warning":
		return zapcore.WarnLevel
	case "error":
		return zapcore.ErrorLevel
	default:
		return zapcore.InfoLevel
	}
}
