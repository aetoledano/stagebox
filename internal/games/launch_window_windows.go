//go:build windows

package games

// Called under curMu, so the job cannot be closed while querying its members.
func localGameWindowPID(process gameProcess, known map[uint32]struct{}) int {
	if job, ok := process.(*jobGameProcess); ok {
		pids, err := jobProcessIDs(job.job)
		if err != nil {
			return 0
		}
		members := make(map[uint32]struct{}, len(pids))
		for _, pid := range pids {
			members[pid] = struct{}{}
		}
		return int(processTreeVisibleWindowPID(members))
	}
	if command, ok := process.(*commandGameProcess); ok && command.trackMode == TrackModeProcessTree {
		running := processTreeRunning(known)
		for pid := range running {
			known[pid] = struct{}{}
		}
		return int(processTreeVisibleWindowPID(running))
	}
	return int(processTreeVisibleWindowPID(map[uint32]struct{}{uint32(process.PID()): {}}))
}
