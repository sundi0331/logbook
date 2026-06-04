package logging

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/sundi0331/logbook/config"
)

func New(cfg config.LogConfig) (*slog.Logger, func() error, error) {
	level, err := parseLevel(cfg.Level)
	if err != nil {
		return nil, nil, err
	}

	writer, cleanup, err := output(cfg)
	if err != nil {
		return nil, nil, err
	}

	opts := &slog.HandlerOptions{Level: level}
	var handler slog.Handler
	switch strings.ToLower(cfg.Format) {
	case "json":
		handler = slog.NewJSONHandler(writer, opts)
	case "text":
		handler = slog.NewTextHandler(writer, opts)
	default:
		_ = cleanup()
		return nil, nil, fmt.Errorf("unsupported log format %q: expected json or text", cfg.Format)
	}

	return slog.New(handler), cleanup, nil
}

func parseLevel(value string) (slog.Level, error) {
	switch strings.ToLower(value) {
	case "debug":
		return slog.LevelDebug, nil
	case "", "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return slog.LevelInfo, fmt.Errorf("unsupported log level %q: expected debug, info, warn, or error", value)
	}
}

func output(cfg config.LogConfig) (io.Writer, func() error, error) {
	switch strings.ToLower(cfg.Out) {
	case "", "stdout":
		return os.Stdout, func() error { return nil }, nil
	case "stderr":
		return os.Stderr, func() error { return nil }, nil
	case "file":
		file, err := os.OpenFile(cfg.Filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o640)
		if err != nil {
			return nil, nil, fmt.Errorf("open log file %q: %w", cfg.Filename, err)
		}
		return file, func() error {
			if err := file.Sync(); err != nil {
				_ = file.Close()
				return err
			}
			return file.Close()
		}, nil
	default:
		return nil, nil, fmt.Errorf("unsupported log output %q: expected stdout, stderr, or file", cfg.Out)
	}
}
