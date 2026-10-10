//go:build !windows

package games

// Native game-window observation is available only on Windows. Keep the
// existing process-start handoff for other platforms.
func localGameWindowPID(process gameProcess, _ map[uint32]struct{}) int {
	return process.PID()
}
