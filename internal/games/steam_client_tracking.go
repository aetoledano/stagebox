package games

import "time"

const steamClientRegistrationGrace = time.Second

const steamClientStaleGrace = 5 * time.Second

// Recovery requires the same dead registration and the same retained warm
// client for the whole grace. A restart/PID reuse cannot inherit permission.
type steamStaleClientRecovery struct {
	stalePID uint32
	client   steamClientRegistration
	since    time.Time
}

func (s *steamStaleClientRecovery) observe(now time.Time, stalePID uint32, client steamClientRegistration) bool {
	if stalePID == 0 || client.pid == 0 {
		*s = steamStaleClientRecovery{}
		return false
	}
	if s.since.IsZero() || s.stalePID != stalePID || s.client != client {
		s.stalePID, s.client, s.since = stalePID, client, now
		return false
	}
	return now.Sub(s.since) >= steamClientStaleGrace
}

// Include creation time, not just PID: Steam can restart during bootstrap,
// and Windows can reuse the old PID. DLL paths can change during an update.
type steamClientRegistration struct {
	pid     uint32
	created uint64
	dll32   string
	dll64   string
}

type steamClientReadiness struct {
	registration steamClientRegistration
	stableSince  time.Time
}

func (s *steamClientReadiness) observe(now time.Time, registration steamClientRegistration) bool {
	if registration.pid == 0 {
		*s = steamClientReadiness{}
		return false
	}
	if s.stableSince.IsZero() || s.registration != registration {
		s.registration, s.stableSince = registration, now
		return false
	}
	return now.Sub(s.stableSince) >= steamClientRegistrationGrace
}
