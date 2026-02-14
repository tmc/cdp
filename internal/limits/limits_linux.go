//go:build linux
// +build linux

package limits

import (
	"fmt"

	"golang.org/x/sys/unix"
)

// setProcLimit sets the process limit on Linux
func setProcLimit(maxProcesses uint64) error {
	if maxProcesses > 0 {
		procLimit := &unix.Rlimit{
			Cur: maxProcesses,
			Max: maxProcesses,
		}

		if err := unix.Setrlimit(unix.RLIMIT_NPROC, procLimit); err != nil {
			return fmt.Errorf("setting process limit: %w", err)
		}
	}
	return nil
}
