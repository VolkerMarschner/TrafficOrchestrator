package logging

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// tempLogPath returns a path inside a temp directory and registers cleanup.
func tempLogPath(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	return filepath.Join(dir, "test.log")
}

func TestNewLogger_CreatesFile(t *testing.T) {
	path := tempLogPath(t)
	logger, err := NewLogger(path, 1, 3)
	if err != nil {
		t.Fatalf("NewLogger returned unexpected error: %v", err)
	}
	defer logger.Close()

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Errorf("expected log file to exist at %s", path)
	}
}

func TestNewLogger_InvalidPath(t *testing.T) {
	_, err := NewLogger("/nonexistent/path/that/does/not/exist/test.log", 1, 3)
	if err == nil {
		t.Error("expected error for invalid path, got nil")
	}
}

func TestLogger_WritesContent(t *testing.T) {
	path := tempLogPath(t)
	logger, err := NewLogger(path, 1, 3)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	defer logger.Close()

	logger.Info("hello info")
	logger.Warn("hello warn")
	logger.Error("hello error")
	logger.Debug("hello debug")

	// Flush by closing so content is flushed to disk
	if err := logger.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	content := string(data)

	for _, want := range []string{"INFO", "hello info", "WARN", "hello warn", "ERROR", "hello error", "DEBUG", "hello debug"} {
		if !strings.Contains(content, want) {
			t.Errorf("log file missing %q\ncontent:\n%s", want, content)
		}
	}
}

func TestLogger_DefaultsApplied(t *testing.T) {
	path := tempLogPath(t)
	// Pass 0 for both maxSizeMB and maxFiles – should use defaults without error
	logger, err := NewLogger(path, 0, 0)
	if err != nil {
		t.Fatalf("NewLogger with defaults: %v", err)
	}
	defer logger.Close()

	if logger.maxSize != int64(10)*1024*1024 {
		t.Errorf("expected default maxSize 10 MB, got %d", logger.maxSize)
	}
	if logger.maxFiles != 5 {
		t.Errorf("expected default maxFiles 5, got %d", logger.maxFiles)
	}
}

func TestLogger_GetLogPath(t *testing.T) {
	path := tempLogPath(t)
	logger, err := NewLogger(path, 1, 3)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	defer logger.Close()

	if got := logger.GetLogPath(); got != path {
		t.Errorf("GetLogPath() = %q, want %q", got, path)
	}
}

func TestLogger_Rotate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rotate.log")

	// Create logger with a very small max size (1 byte) to force rotation
	logger, err := NewLogger(path, 0, 3)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	// Manually shrink maxSize to force rotation on the next call
	logger.maxSize = 1

	logger.Info("first line that exceeds 1 byte")

	if err := logger.Rotate(); err != nil {
		t.Fatalf("Rotate: %v", err)
	}

	// After rotation the original path should exist as a new (empty) file
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Errorf("expected new log file to exist after rotation at %s", path)
	}
	// The old file should have been renamed to path.1
	if _, err := os.Stat(path + ".1"); os.IsNotExist(err) {
		t.Errorf("expected rotated file to exist at %s.1", path)
	}

	logger.Close()
}

func TestLogger_RotateIdempotentWhenSmall(t *testing.T) {
	path := tempLogPath(t)
	logger, err := NewLogger(path, 10, 3)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	defer logger.Close()

	logger.Info("tiny message")

	// File is well below 10 MB – Rotate should be a no-op
	if err := logger.Rotate(); err != nil {
		t.Errorf("Rotate on small file returned error: %v", err)
	}

	// Rotated file must NOT exist
	if _, err := os.Stat(path + ".1"); err == nil {
		t.Errorf("unexpected rotated file %s.1 after no-op rotation", path)
	}
}
