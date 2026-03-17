//go:build !windows

package update

import (
	"fmt"
	"os"
	"syscall"
)

// applyUpdate atomically replaces currentBinary with tmpFile and exec's the new binary.
// syscall.Exec replaces the current process image — the function never returns on success.
//
// Safety: the original binary is moved to currentBinary+".bak" before the new one is
// placed. If exec fails (e.g. wrong architecture), the original is restored so the agent
// can keep running with its old binary.
func applyUpdate(tmpFile, currentBinary string, restartArgs []string) error {
	backupFile := currentBinary + ".bak"

	// Move the current binary aside so we can restore it if exec fails.
	if err := os.Rename(currentBinary, backupFile); err != nil {
		os.Remove(tmpFile)
		return fmt.Errorf("update: backup %q → %q: %w", currentBinary, backupFile, err)
	}

	// Place the new binary.
	if err := os.Rename(tmpFile, currentBinary); err != nil {
		// New binary couldn't be placed — restore original.
		os.Rename(backupFile, currentBinary) //nolint:errcheck
		return fmt.Errorf("update: rename %q → %q: %w", tmpFile, currentBinary, err)
	}

	// Replace the current process with the new binary (same PID group, same environment).
	argv := append([]string{currentBinary}, restartArgs...)
	if err := syscall.Exec(currentBinary, argv, os.Environ()); err != nil {
		// exec failed (e.g. exec format error on architecture mismatch) — restore original.
		os.Rename(backupFile, currentBinary) //nolint:errcheck
		return fmt.Errorf("update: exec new binary: %w", err)
	}

	// Unreachable on success — the backup is cleaned up by the new process on next startup.
	return nil
}
