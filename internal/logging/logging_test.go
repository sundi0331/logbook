package logging

import (
	"log/slog"
	"os"
	"testing"

	"github.com/sundi0331/logbook/config"
)

func TestNewSupportsConfiguredLevels(t *testing.T) {
	tests := map[string]slog.Level{
		"debug": slog.LevelDebug,
		"info":  slog.LevelInfo,
		"warn":  slog.LevelWarn,
		"error": slog.LevelError,
	}

	for level := range tests {
		t.Run(level, func(t *testing.T) {
			logger, cleanup, err := New(config.LogConfig{Format: "json", Out: "stdout", Level: level})
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}
			if logger == nil {
				t.Fatal("New() returned nil logger")
			}
			if err := cleanup(); err != nil {
				t.Fatalf("cleanup() error = %v", err)
			}
		})
	}
}

func TestNewRejectsUnsupportedLevels(t *testing.T) {
	for _, level := range []string{"trace", "warning"} {
		t.Run(level, func(t *testing.T) {
			_, _, err := New(config.LogConfig{Format: "json", Out: "stdout", Level: level})
			if err == nil {
				t.Fatal("New() error = nil, want unsupported level error")
			}
		})
	}
}

func TestFileOutputWritesLogFile(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "logbook-*.log")
	if err != nil {
		t.Fatal(err)
	}
	path := file.Name()
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	logger, cleanup, err := New(config.LogConfig{Format: "text", Out: "file", Filename: path, Level: "info"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	logger.Info("test message")
	if err := cleanup(); err != nil {
		t.Fatalf("cleanup() error = %v", err)
	}

	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(contents) == 0 {
		t.Fatal("expected log file to contain output")
	}
}
