//go:build windows

package controller

import (
	"context"
	"fmt"
	lgx "launcher/internal/logx"
	"time"
)

type controllerRumbleRequest struct {
	target *controllerTestRequest
	ctx    context.Context
	result chan error
}

// Wails only queues requests; SDL calls stay on the locked polling owner.
func (b *ControllerBridge) TestControllerRumble(id string) error {
	if id == "" {
		b.rumbleTarget.Store(nil)
		return nil
	}
	connected := false
	for _, info := range b.Controllers() {
		if info.ID == id && info.Connected {
			connected = true
			break
		}
	}
	if !connected {
		return fmt.Errorf("controller is disconnected")
	}
	target := &controllerTestRequest{id: id}
	ctx, cancel := context.WithTimeout(b.ctx, time.Second)
	defer cancel()
	request := &controllerRumbleRequest{target: target, ctx: ctx, result: make(chan error, 1)}
	b.rumbleTarget.Store(target)
	if !b.testRumble.CompareAndSwap(nil, request) {
		b.rumbleTarget.CompareAndSwap(target, nil)
		return fmt.Errorf("a rumble test is already pending")
	}
	defer b.testRumble.CompareAndSwap(request, nil)
	select {
	case err := <-request.result:
		return err
	case <-ctx.Done():
		return fmt.Errorf("controller rumble test interrupted: %w", ctx.Err())
	}
}

func (b *ControllerBridge) pollTestRumble(s *sdlBackend, now time.Time) {
	target := b.rumbleTarget.Load()
	if s != nil && s.testRumbleID != 0 && (target != s.testRumbleOwner || !now.Before(s.testRumbleUntil) || b.virtualInput.Load() != nil) {
		s.stopGamepadRumble(s.testRumbleID, s.gamepads[s.testRumbleID])
	}
	request := b.testRumble.Swap(nil)
	if request == nil {
		return
	}
	var err error
	switch {
	case request.ctx.Err() != nil:
		err = request.ctx.Err()
	case target == nil || target != request.target:
		err = fmt.Errorf("controller rumble test was cancelled")
	case s == nil:
		err = fmt.Errorf("SDL controller input is unavailable")
	case b.virtualInput.Load() != nil:
		err = fmt.Errorf("close the virtual controller session before testing rumble")
	default:
		err = s.testPhysicalRumble(target, now)
	}
	request.result <- err
}

func (s *sdlBackend) testPhysicalRumble(test *controllerTestRequest, now time.Time) error {
	for id, info := range s.info {
		if info.ID != test.id {
			continue
		}
		handle := s.gamepads[id]
		connected, _, _ := s.connected.Call(handle)
		if connected&0xff == 0 {
			break
		}
		// A moderate, finite pulse works without a game, driver setup or HidHide.
		result, _, _ := s.rumble.Call(handle, 32768, 32768, 600)
		if result&0xff == 0 {
			err := fmt.Errorf("SDL could not rumble this controller: %s", s.errorText())
			lgx.Warnf("controller rumble test failed id=%s name=%q: %v", test.id, info.Name, err)
			return err
		}
		s.testRumbleID, s.testRumbleOwner, s.testRumbleUntil = id, test, now.Add(600*time.Millisecond)
		lgx.Infof("controller rumble test accepted id=%s name=%q low=32768 high=32768 duration=600ms", test.id, info.Name)
		return nil
	}
	return fmt.Errorf("controller disconnected; reconnect it and reopen the mapping test")
}
