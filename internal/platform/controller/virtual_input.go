package controller

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// The SDL owner publishes snapshots; a separate worker owns IPC and teardown.
// Pin the physical path before the helper can create a virtual device.
type virtualInputSession struct {
	sourcePaths  []string
	ctx          context.Context
	cancel       context.CancelFunc
	pinned       chan struct{}
	pinOnce      sync.Once
	frames       chan []xinputGamepad
	active       atomic.Bool
	inputGates   []launcherInputGate // SDL owner only
	instances    []uint32            // SDL owner only; reconnect must rearm independently
	conn         net.Conn
	done         chan struct{}
	closeOnce    sync.Once
	closeProcess func() error
	mu           sync.Mutex
	err          error
}

func newVirtualInputSession(ctx context.Context, path string, more ...string) *virtualInputSession {
	ctx, cancel := context.WithCancel(ctx)
	paths := append([]string{path}, more...)
	return &virtualInputSession{sourcePaths: paths, inputGates: make([]launcherInputGate, len(paths)), instances: make([]uint32, len(paths)), ctx: ctx, cancel: cancel, pinned: make(chan struct{}), frames: make(chan []xinputGamepad, 1), done: make(chan struct{})}
}

func (s *virtualInputSession) publish(state xinputGamepad) {
	s.publishStates([]xinputGamepad{state})
}

func (s *virtualInputSession) publishDevices(devices []controllerDevice) {
	states := make([]xinputGamepad, len(s.sourcePaths))
	present := make([]bool, len(states))
	for i, path := range s.sourcePaths {
		instance := uint32(0)
		for _, device := range devices {
			if device.path == path {
				states[i], present[i], instance = device.state, true, device.instance
				break
			}
		}
		if s.instances[i] != instance {
			s.inputGates[i].reset()
			s.instances[i] = instance
		}
	}
	s.publishPresentStates(states, present)
}

func (s *virtualInputSession) publishStates(states []xinputGamepad) {
	present := make([]bool, len(states))
	for i := range present {
		present[i] = true
	}
	s.publishPresentStates(states, present)
}

func (s *virtualInputSession) resetGates() {
	for i := range s.inputGates {
		s.inputGates[i].reset()
	}
}

func (s *virtualInputSession) publishPresentStates(states []xinputGamepad, present []bool) {
	for i, state := range states {
		if !s.inputGates[i].allow(s.active.Load() && present[i], gamepadNeutral(state)) {
			states[i] = xinputGamepad{}
		}
	}
	select {
	case s.frames <- states:
	default:
		select {
		case <-s.frames:
		default:
		}
		select {
		case s.frames <- states:
		default:
		}
	}
}

func (s *virtualInputSession) attach(conn net.Conn, closeProcess func() error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ctx.Err(); err != nil {
		conn.Close()
		closeProcess()
		return err
	}
	s.conn, s.closeProcess = conn, closeProcess
	s.active.Store(true)
	go s.run()
	return nil
}

func virtualReport(state xinputGamepad) [12]byte {
	var data [12]byte
	binary.LittleEndian.PutUint16(data[:2], state.wButtons)
	data[2], data[3] = state.bLeftTrigger, state.bRightTrigger
	for i, value := range []int16{state.sThumbLX, state.sThumbLY, state.sThumbRX, state.sThumbRY} {
		binary.LittleEndian.PutUint16(data[4+i*2:], uint16(value))
	}
	return data
}

func (s *virtualInputSession) run() {
	defer func() {
		s.active.Store(false)
		s.conn.Close()
		if err := s.closeProcess(); err != nil {
			s.setError(err)
		}
		close(s.done)
	}()
	heartbeat := time.NewTicker(100 * time.Millisecond)
	defer heartbeat.Stop()
	last := time.Time{}
	states := make([]xinputGamepad, len(s.sourcePaths))
	for {
		select {
		case <-s.ctx.Done():
			return
		case states = <-s.frames:
			last = time.Now()
		case <-heartbeat.C:
			if time.Since(last) > 200*time.Millisecond {
				states = make([]xinputGamepad, len(s.sourcePaths))
			}
		}
		data := make([]byte, 0, len(states)*12)
		for _, state := range states {
			report := virtualReport(state)
			data = append(data, report[:]...)
		}
		if err := s.conn.SetWriteDeadline(time.Now().Add(500 * time.Millisecond)); err != nil {
			if s.ctx.Err() == nil {
				s.setError(err)
			}
			return
		}
		if count, err := s.conn.Write(data); err != nil || count != len(data) {
			if s.ctx.Err() != nil {
				return
			}
			if err == nil {
				err = io.ErrShortWrite
			}
			s.setError(fmt.Errorf("virtual input connection lost: %w", err))
			return
		}
	}
}

func (s *virtualInputSession) setError(err error) { s.mu.Lock(); s.err = err; s.mu.Unlock() }

func (s *virtualInputSession) close() error {
	s.closeOnce.Do(func() {
		s.active.Store(false)
		s.cancel()
		s.mu.Lock()
		conn := s.conn
		s.mu.Unlock()
		if conn != nil {
			conn.Close()
			<-s.done
		}
	})
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}
