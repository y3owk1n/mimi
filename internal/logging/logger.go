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
)

func New(cfg *config.Config) *zap.SugaredLogger {
	level := parseLevel(cfg.Settings.LogLevel)

	var consoleWriter zapcore.WriteSyncer
	if consoleWriter == nil {
		consoleWriter = os.Stdout
	}

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
		w := &lumberjack.Logger{
			Filename:   expandHome(cfg.Settings.LogFile),
			MaxSize:    100,
			MaxBackups: 3,
			MaxAge:     28,
		}

		// Create file encoder (JSON for machine parsing)
		fileEncoder := zapcore.NewJSONEncoder(fileEncoderConfig)

		// Add file core
		cores = append(cores, zapcore.NewCore(fileEncoder, zapcore.AddSync(w), level))
	}

	core := zapcore.NewTee(cores...)

	return zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel)).Sugar()
}

func WriteEventLog(ctx context.Context, sub events.Subscriber, logPath string, logger *zap.SugaredLogger) {
	if logPath == "" {
		return
	}

	eventLogPath := logPath + ".events.jsonl"

	f, err := openAppend(eventLogPath)
	if err != nil {
		logger.Warnw("cannot open event log", "err", err)

		return
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	for {
		select {
		case <-ctx.Done():
			return
		case e, ok := <-sub:
			if !ok {
				return
			}

			if err := enc.Encode(e); err != nil {
				logger.Warnw("event log write error", "err", err)
			}
		}
	}
}

func openAppend(path string) (*os.File, error) {
	path = expandHome(path)

	err := os.MkdirAll(filepath.Dir(path), 0o755)
	if err != nil {
		return nil, err
	}

	return os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~") {
		home, _ := os.UserHomeDir()

		return filepath.Join(home, path[1:])
	}

	return path
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
