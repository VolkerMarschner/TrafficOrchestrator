package logging

import (
	"fmt"
	"os"
	"sync"
	"time"
)

// Logger implements a simple rotating file logger.
type Logger struct {
	file     *os.File
	path     string
	maxSize  int64 // max bytes per file before rotation
	maxFiles int   // keep this many rotated files
	mu       sync.Mutex
}

// NewLogger creates a new rotating file logger.
func NewLogger(path string, maxSizeMB int, maxFiles int) (*Logger, error) {
	if maxSizeMB <= 0 {
		maxSizeMB = 10 // default 10 MB
	}
	if maxFiles <= 0 {
		maxFiles = 5 // default keep 5 files
	}

	logger := &Logger{
		path:     path,
		maxSize:  int64(maxSizeMB) * 1024 * 1024,
		maxFiles: maxFiles,
	}

	if err := logger.openFile(); err != nil {
		return nil, fmt.Errorf("failed to open log file: %w", err)
	}

	return logger, nil
}

// openFileLocked opens the log file for appending. Caller must hold l.mu.
func (l *Logger) openFileLocked() error {
	if l.file != nil {
		l.file.Close()
		l.file = nil
	}

	file, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	l.file = file
	return nil
}

// openFile opens the log file for appending.
func (l *Logger) openFile() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	return l.openFileLocked()
}

// rotateLocked checks if rotation is needed and rotates the log file.
// Caller must hold l.mu. (H3: extracted so Log() can call it without deadlock)
func (l *Logger) rotateLocked() error {
	if l.file == nil {
		return l.openFileLocked()
	}

	info, err := l.file.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat log file: %w", err)
	}

	if info.Size() < l.maxSize {
		return nil // no rotation needed yet
	}

	if err := l.file.Close(); err != nil {
		return fmt.Errorf("failed to close log file: %w", err)
	}
	l.file = nil

	// Rotate existing files: .N → .(N+1)
	for i := l.maxFiles - 1; i > 0; i-- {
		oldPath := fmt.Sprintf("%s.%d", l.path, i)
		newPath := fmt.Sprintf("%s.%d", l.path, i+1)
		if _, err := os.Stat(oldPath); err == nil {
			if err := os.Rename(oldPath, newPath); err != nil {
				return fmt.Errorf("failed to rename %s: %w", oldPath, err)
			}
		}
	}

	// Move current file to .1
	if err := os.Rename(l.path, fmt.Sprintf("%s.1", l.path)); err != nil {
		return fmt.Errorf("failed to rename current log: %w", err)
	}

	// Open a fresh log file — must call locked variant to avoid deadlock.
	return l.openFileLocked()
}

// Rotate is the public API that checks for rotation and rotates if needed.
func (l *Logger) Rotate() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.rotateLocked()
}

// Log writes a formatted log message with timestamp.
// It checks for log rotation before each write. (H3)
func (l *Logger) Log(level, message string) {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	logLine := fmt.Sprintf("[%s] %s: %s\n", timestamp, level, message)

	l.mu.Lock()
	defer l.mu.Unlock()

	// Rotate if the file has reached maxSize. Errors are best-effort.
	_ = l.rotateLocked()

	if l.file != nil {
		if _, err := l.file.WriteString(logLine); err != nil {
			// Fall back to stderr so the message is not silently dropped
			fmt.Fprint(os.Stderr, logLine)
		}
	} else {
		fmt.Fprint(os.Stderr, logLine) // Fallback to stderr if file not open
	}
}

// Info logs an informational message.
func (l *Logger) Info(message string) {
	l.Log("INFO", message)
}

// Error logs an error message.
func (l *Logger) Error(message string) {
	l.Log("ERROR", message)
}

// Warn logs a warning message.
func (l *Logger) Warn(message string) {
	l.Log("WARN", message)
}

// Debug logs a debug message.
func (l *Logger) Debug(message string) {
	l.Log("DEBUG", message)
}

// Close closes the log file.
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.file != nil {
		return l.file.Close()
	}
	return nil
}

// GetLogPath returns the current log file path.
func (l *Logger) GetLogPath() string {
	return l.path
}
