package games

import "time"

const (
	steamPollInterval = 500 * time.Millisecond
	steamRestartGrace = 10 * time.Second
)

type steamSessionState struct {
	absentSince time.Time
}

func (s *steamSessionState) finished(now time.Time, clients int, visibleGame bool) bool {
	if clients > 0 || visibleGame {
		s.absentSince = time.Time{}
		return false
	}
	if s.absentSince.IsZero() {
		s.absentSince = now
	}
	return now.Sub(s.absentSince) >= steamRestartGrace
}

type steamProcessLifetime struct {
	created uint64
	exited  uint64
}

func (p steamProcessLifetime) canParent(childCreated uint64) bool {
	return childCreated >= p.created && (p.exited == 0 || childCreated <= p.exited)
}
