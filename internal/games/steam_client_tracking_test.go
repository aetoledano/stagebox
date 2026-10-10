package games

import (
	"testing"
	"time"
)

func TestSteamClientReadinessSurvivesOnlyContinuousRegistration(t *testing.T) {
	start := time.Unix(100, 0)
	client := steamClientRegistration{pid: 42, created: 1, dll32: "client32", dll64: "client64"}
	var state steamClientReadiness
	if state.observe(start, client) || state.observe(start.Add(steamClientRegistrationGrace/2), client) {
		t.Fatal("a newly registered client must not trigger game launch")
	}
	if !state.observe(start.Add(steamClientRegistrationGrace), client) {
		t.Fatal("a stable registered client should allow game launch")
	}
	state.observe(start.Add(2*steamClientRegistrationGrace), steamClientRegistration{})
	if state.observe(start.Add(3*steamClientRegistrationGrace), client) {
		t.Fatal("registration disappearing must discard prior readiness")
	}
	if !state.observe(start.Add(4*steamClientRegistrationGrace), client) {
		t.Fatal("a recovered client should become ready after stabilizing again")
	}
}

func TestSteamClientReadinessResetsOnRestartOrDLLChange(t *testing.T) {
	start := time.Unix(100, 0)
	client := steamClientRegistration{pid: 42, created: 1, dll32: "client32", dll64: "client64"}
	for _, changed := range []steamClientRegistration{
		{pid: 43, created: 2, dll32: client.dll32, dll64: client.dll64},
		{pid: client.pid, created: 2, dll32: client.dll32, dll64: client.dll64},
		{pid: client.pid, created: client.created, dll32: "updated32", dll64: client.dll64},
		{pid: client.pid, created: client.created, dll32: client.dll32, dll64: "updated64"},
	} {
		var state steamClientReadiness
		state.observe(start, client)
		if state.observe(start.Add(2*steamClientRegistrationGrace), changed) {
			t.Fatal("a replacement process or DLL registration must not inherit readiness")
		}
		if !state.observe(start.Add(3*steamClientRegistrationGrace), changed) {
			t.Fatal("the replacement should become ready after stabilizing")
		}
	}
}

func TestSteamStaleRecoveryRequiresContinuousSameDeadPIDAndWarmClient(t *testing.T) {
	start := time.Unix(100, 0)
	warm := steamClientRegistration{pid: 9720, created: 1}
	var recovery steamStaleClientRecovery
	if recovery.observe(start, 3088, warm) || recovery.observe(start.Add(steamClientStaleGrace-time.Millisecond), 3088, warm) {
		t.Fatal("a transient registration mismatch must not restart Steam")
	}
	if !recovery.observe(start.Add(steamClientStaleGrace), 3088, warm) {
		t.Fatal("persistent dead registration should permit guarded recovery")
	}
	for _, changed := range []struct {
		dead   uint32
		client steamClientRegistration
	}{
		{0, warm},                         // A non-stale/valid/missing/uncertain registry observation.
		{3088, steamClientRegistration{}}, // Cold, ambiguous or replaced client.
		{1234, warm},                      // Another stale PID starts a fresh grace.
		{3088, steamClientRegistration{pid: warm.pid, created: 2}}, // PID reuse.
		{3088, steamClientRegistration{pid: 5000, created: 2}},     // Restart.
	} {
		var state steamStaleClientRecovery
		state.observe(start, 3088, warm)
		if state.observe(start.Add(2*steamClientStaleGrace), changed.dead, changed.client) {
			t.Fatal("changed or uncertain evidence inherited recovery permission")
		}
		if state.observe(start.Add(2*steamClientStaleGrace+time.Millisecond), 3088, warm) {
			t.Fatal("restored evidence must stabilize again")
		}
	}
}
