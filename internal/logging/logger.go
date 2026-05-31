package logging

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/y3owk1n/mimi/internal/config"
	"github.com/y3owk1n/mimi/internal/events"
)

func New(cfg *config.Config) *slog.Logger {
	level := parseLevel(cfg.Settings.LogLevel)

	var handler slog.Handler

	w := openLogFile(cfg.Settings.LogFile)
	if cfg.Settings.LogFormat == "json" {
		handler = slog.NewJSONHandler(io.MultiWriter(os.Stderr, w),
			&slog.HandlerOptions{Level: level})
	} else {
		handler = slog.NewTextHandler(io.MultiWriter(os.Stderr, w),
			&slog.HandlerOptions{Level: level})
	}

	return slog.New(handler)
}

func WriteEventLog(
	ctx context.Context,
	sub events.Subscriber,
	logPath string,
	logger *slog.Logger,
) {
	eventLogPath := logPath + ".events.jsonl"

	f, err := openAppend(eventLogPath)
	if err != nil {
		logger.Warn("cannot open event log", "err", err)

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

			err := enc.Encode(e)
			if err != nil {
				logger.Warn("event log write error", "err", err)
			}
		}
	}
}

func openLogFile(path string) *os.File {
	path = expandHome(path)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return os.Stderr
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return os.Stderr
	}

	return f
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

func parseLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
