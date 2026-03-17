//go:build windows

package update

import (
	"fmt"
	"os"
	"os/exec"
)

// updateBat is a batch script that waits for the current process to exit,
// swaps the binary files, and restarts the new binary.
// %s placeholders: tmpFile, currentBinary, currentBinary, restartArgs.
// Note: arguments containing spaces or special batch characters are not supported.
// In practice this is not an issue because agents persist their configuration to
// to.conf on first run, so restartArgs is typically empty on self-update. (N3)
const updateBat = `@echo off
timeout /t 3 /nobreak > nul
move /y "%s" "%s"
if errorlevel 1 (
    echo Update failed: could not replace binary
    exit /b 1
)
start "" "%s" %s
del "%%~f0"
`

// applyUpdate writes a helper batch script that swaps the new binary into place
// after this process exits, then exits the current process.
// The batch script deletes itself on completion.
func applyUpdate(tmpFile, currentBinary string, restartArgs []string) error {
	batPath := currentBinary + "_update.bat"

	// Build a simple space-separated argument string for the batch script.
	// Arguments with spaces or special characters are not supported — see comment on updateBat.
	argStr := ""
	for _, arg := range restartArgs {
		argStr += " " + arg
	}

	script := fmt.Sprintf(updateBat, tmpFile, currentBinary, currentBinary, argStr)

	if err := os.WriteFile(batPath, []byte(script), 0755); err != nil {
		os.Remove(tmpFile)
		return fmt.Errorf("update: write helper script: %w", err)
	}

	cmd := exec.Command("cmd", "/c", "start", "", "/b", batPath)
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		os.Remove(tmpFile)
		os.Remove(batPath)
		return fmt.Errorf("update: launch helper script: %w", err)
	}

	fmt.Println("Update downloaded. Restarting via helper script...")
	os.Exit(0)
	return nil // unreachable
}
