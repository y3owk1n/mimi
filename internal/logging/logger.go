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

// New creates a zap sugared logger with console and optional file output.
func New(cfg *config.Config) *zap.SugaredLogger {
	level := parseLevel(cfg.Settings.LogLevel)

	consoleWriter := zapcore.WriteSyncer(os.Stdout)

	isTerminal := false

	if f, ok := consoleWriter.(*os.File); ok {
		isTerminal = term.IsTerminal(int(f.Fd()))
	}

	// Configure encoder
	consoleEncoderConfig := zap.NewDevelopmentEncoderConfig()

	consoleEncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	if isTerminal {
		consoleEncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	} else {
		consoleEncoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder
	}

	fileEncoderConfig := zap.NewProductionEncoderConfig()

	fileEncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	fileEncoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder

	// Create console encoder (human-readable)
	consoleEncoder := zapcore.NewConsoleEncoder(consoleEncoderConfig)

	// Create cores slice
	cores := []zapcore.Core{
		zapcore.NewCore(consoleEncoder, zapcore.AddSync(consoleWriter), level),
	}

	if cfg.Settings.LogFile != "" {
		logWriter := &lumberjack.Logger{
			Filename:   paths.ExpandHome(cfg.Settings.LogFile),
			MaxSize:    logMaxSizeMB,
			MaxBackups: logMaxBackups,
			MaxAge:     logMaxAgeDays,
		}

		// Create file encoder (JSON for machine parsing)
		fileEncoder := zapcore.NewJSONEncoder(fileEncoderConfig)

		// Add file core
		cores = append(cores, zapcore.NewCore(fileEncoder, zapcore.AddSync(logWriter), level))
	}

	core := zapcore.NewTee(cores...)

	return zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel)).Sugar()
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
