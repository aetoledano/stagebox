//go:build !windows

package games

import "os"

func killProcessTree(process *os.Process) error {
	if process == nil {
		return nil
	}
	return process.Kill()
}
