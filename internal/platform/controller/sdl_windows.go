//go:build windows

package controller

import (
	"fmt"
	lgx "launcher/internal/logx"
	"runtime"
	"slices"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const sdlInitGamepad = 0x00002000

// All SDL lifetime/event/input calls are owned by the bridge's locked OS
// thread. Only the bridge's atomic connected count crosses into App status.
type sdlBackend struct {
	dll                                                             *windows.DLL
	init, quit, setMainReady, setHint, getError, getVersion         *windows.Proc
	pump, flush, getGamepads, free, open, close, connected          *windows.Proc
	button, axis, name, ioFromFile, addMappings                     *windows.Proc
	ioFromConstMem                                                  *windows.Proc
	mapping, getJoysticks, joystickName, vendor, product, isGamepad *windows.Proc
	gamepads                                                        map[uint32]uintptr
	joysticks                                                       map[uint32]bool
	activeID                                                        uint32
	nextDiscovery                                                   time.Time
	failures                                                        int
	initialized                                                     bool
	mappingPaths                                                    []string
	mappingFiles                                                    []controllerMappingFile
	mappingHash, pendingMappingHash                                 [32]byte
	mappingPending                                                  bool
	nextMappingCheck                                                time.Time
	mappingError                                                    string
	mappingRevision                                                 uint64
}

func openSDLBackend() (*sdlBackend, error) {
	path, err := sdlRuntimePath()
	if err != nil {
		return nil, fmt.Errorf("extract SDL3 runtime: %w", err)
	}
	handle, err := windows.LoadLibraryEx(path, 0, windows.LOAD_LIBRARY_SEARCH_DLL_LOAD_DIR|windows.LOAD_LIBRARY_SEARCH_SYSTEM32)
	if err != nil {
		return nil, fmt.Errorf("load SDL3 runtime: %w", err)
	}
	s := &sdlBackend{dll: &windows.DLL{Name: path, Handle: handle}, gamepads: make(map[uint32]uintptr), joysticks: make(map[uint32]bool)}
	ready := false
	defer func() {
		if !ready {
			s.shutdown()
		}
	}()
	for _, binding := range []struct {
		name   string
		target **windows.Proc
	}{
		{"SDL_Init", &s.init}, {"SDL_Quit", &s.quit}, {"SDL_SetMainReady", &s.setMainReady},
		{"SDL_SetHint", &s.setHint}, {"SDL_GetError", &s.getError}, {"SDL_GetVersion", &s.getVersion},
		{"SDL_PumpEvents", &s.pump}, {"SDL_FlushEvents", &s.flush}, {"SDL_GetGamepads", &s.getGamepads},
		{"SDL_free", &s.free}, {"SDL_OpenGamepad", &s.open}, {"SDL_CloseGamepad", &s.close},
		{"SDL_GamepadConnected", &s.connected}, {"SDL_GetGamepadButton", &s.button},
		{"SDL_GetGamepadAxis", &s.axis}, {"SDL_GetGamepadName", &s.name},
		{"SDL_IOFromFile", &s.ioFromFile}, {"SDL_AddGamepadMappingsFromIO", &s.addMappings},
		{"SDL_IOFromConstMem", &s.ioFromConstMem},
		{"SDL_GetGamepadMapping", &s.mapping}, {"SDL_GetJoysticks", &s.getJoysticks},
		{"SDL_GetJoystickNameForID", &s.joystickName}, {"SDL_GetJoystickVendorForID", &s.vendor},
		{"SDL_GetJoystickProductForID", &s.product}, {"SDL_IsGamepad", &s.isGamepad},
	} {
		proc, err := s.dll.FindProc(binding.name)
		if err != nil {
			return nil, fmt.Errorf("bind SDL3 %s: %w", binding.name, err)
		}
		*binding.target = proc
	}
	s.setMainReady.Call()
	// The Wails window is not an SDL window. Read background input for existing
	// game mappings, pairing/pointer helpers and the deliberate kill shortcut;
	// the existing native foreground gate still owns launcher navigation.
	if err := s.hint("SDL_JOYSTICK_ALLOW_BACKGROUND_EVENTS", "1"); err != nil {
		return nil, err
	}
	result, _, _ := s.init.Call(sdlInitGamepad)
	if result&0xff == 0 {
		// SDL_Init rolls back initialized subsystems on failure.
		return nil, fmt.Errorf("initialize SDL3 gamepads: %s", s.errorText())
	}
	s.initialized = true
	version, _, _ := s.getVersion.Call()
	lgx.Infof("controller backend SDL3 initialized version=%d.%d.%d runtime=%q", version/1000000, version/1000%1000, version%1000, path)
	s.loadMappings()
	ready = true
	return s, nil
}

func (s *sdlBackend) hint(name, value string) error {
	n, err := windows.BytePtrFromString(name)
	if err != nil {
		return err
	}
	v, err := windows.BytePtrFromString(value)
	if err != nil {
		return err
	}
	result, _, _ := s.setHint.Call(uintptr(unsafe.Pointer(n)), uintptr(unsafe.Pointer(v)))
	runtime.KeepAlive(n)
	runtime.KeepAlive(v)
	if result&0xff == 0 {
		return fmt.Errorf("set SDL3 hint %s: %s", name, s.errorText())
	}
	return nil
}

func (s *sdlBackend) errorText() string {
	ptr, _, _ := s.getError.Call()
	if ptr == 0 {
		return "unknown SDL3 error"
	}
	return windows.BytePtrToString((*byte)(unsafe.Pointer(ptr)))
}

func (s *sdlBackend) loadMappings() {
	if path, err := bundledSDLMappingPath(); err != nil {
		lgx.Warnf("extract bundled SDL3 controller mappings: %v", err)
	} else {
		if err := s.loadMappingFile(path, "bundled"); err != nil {
			lgx.Warnf("load bundled SDL3 controller mappings: %v", err)
		}
	}
	paths, err := controllerDatabasePaths()
	if err != nil {
		lgx.Warnf("resolve custom SDL3 controller mappings: %v", err)
		return
	}
	s.mappingPaths = paths
	files, hash, err := readControllerMappingFiles(paths)
	if err != nil {
		s.reportMappingError(err)
		return
	}
	for _, file := range files {
		if err := s.loadMappingData(file); err != nil {
			s.reportMappingError(err)
			return
		}
	}
	s.mappingFiles, s.mappingHash = files, hash
}

func (s *sdlBackend) loadMappingFile(path, source string) error {
	file, err := windows.BytePtrFromString(path)
	if err != nil {
		return err
	}
	mode, _ := windows.BytePtrFromString("r")
	stream, _, _ := s.ioFromFile.Call(uintptr(unsafe.Pointer(file)), uintptr(unsafe.Pointer(mode)))
	runtime.KeepAlive(file)
	runtime.KeepAlive(mode)
	if stream == 0 {
		return fmt.Errorf("open SDL3 controller mappings path=%q err=%s", path, s.errorText())
	}
	// closeio=true transfers stream ownership to SDL, even on failure.
	result, _, _ := s.addMappings.Call(stream, 1)
	if int32(result) < 0 {
		return fmt.Errorf("load SDL3 controller mappings path=%q err=%s", path, s.errorText())
	}
	lgx.Infof("SDL3 controller mappings loaded source=%s path=%q count=%d", source, path, int32(result))
	return nil
}

func (s *sdlBackend) loadMappingData(file controllerMappingFile) error {
	data := file.data
	// SDL's text reader need not handle the BOM produced by some Windows editors.
	if len(data) >= 3 && string(data[:3]) == "\xef\xbb\xbf" {
		data = data[3:]
	}
	if len(data) == 0 {
		return nil
	}
	stream, _, _ := s.ioFromConstMem.Call(uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)))
	if stream == 0 {
		return fmt.Errorf("open custom SDL3 mapping snapshot path=%q err=%s", file.path, s.errorText())
	}
	result, _, _ := s.addMappings.Call(stream, 1)
	runtime.KeepAlive(data)
	if int32(result) < 0 {
		return fmt.Errorf("load custom SDL3 mapping snapshot path=%q err=%s", file.path, s.errorText())
	}
	lgx.Infof("SDL3 controller mappings loaded source=custom path=%q count=%d", file.path, int32(result))
	return nil
}

func (s *sdlBackend) reportMappingError(err error) {
	if message := err.Error(); message != s.mappingError {
		lgx.Warnf("SDL3 controller mapping reload deferred: %s; retaining last loaded mappings", message)
		s.mappingError = message
	}
}

func (s *sdlBackend) checkMappingChanges(now time.Time) error {
	if len(s.mappingPaths) == 0 || now.Before(s.nextMappingCheck) {
		return nil
	}
	s.nextMappingCheck = now.Add(time.Second)
	files, hash, err := readControllerMappingFiles(s.mappingPaths)
	if err != nil {
		s.mappingPending = false
		s.reportMappingError(err)
		return nil
	}
	if hash == s.mappingHash {
		s.mappingPending = false
		s.mappingError = ""
		return nil
	}
	// Two matching snapshots avoid reloading a truncated/in-progress editor save.
	if !s.mappingPending || hash != s.pendingMappingHash {
		s.pendingMappingHash, s.mappingPending = hash, true
		return nil
	}
	s.mappingPending = false
	s.mappingRevision++
	if err := s.rebuildMappings(files); err != nil {
		s.reportMappingError(err)
		if restoreErr := s.rebuildMappings(s.mappingFiles); restoreErr != nil {
			return fmt.Errorf("restore SDL3 mappings after reload failure: %w", restoreErr)
		}
		return nil
	}
	s.mappingFiles, s.mappingHash, s.mappingError = files, hash, ""
	lgx.Info("SDL3 controller mappings reloaded; waiting for neutral controller input")
	return nil
}

// SDL's mapping loader is additive. Reinitialize just the SDL input subsystem
// on its owner thread so deleted rows/files also restore the bundled defaults.
func (s *sdlBackend) rebuildMappings(files []controllerMappingFile) error {
	s.closeInput()
	if err := s.hint("SDL_JOYSTICK_ALLOW_BACKGROUND_EVENTS", "1"); err != nil {
		return err
	}
	result, _, _ := s.init.Call(sdlInitGamepad)
	if result&0xff == 0 {
		return fmt.Errorf("reinitialize SDL3 input: %s", s.errorText())
	}
	s.initialized = true
	path, err := bundledSDLMappingPath()
	if err != nil {
		return err
	}
	if err := s.loadMappingFile(path, "bundled"); err != nil {
		return err
	}
	for _, file := range files {
		if err := s.loadMappingData(file); err != nil {
			return err
		}
	}
	return nil
}

func (s *sdlBackend) poll(now time.Time) (xinputGamepad, uint32, int, error) {
	if err := s.checkMappingChanges(now); err != nil {
		return xinputGamepad{}, 0, 0, err
	}
	s.pump.Call()
	// State polling does not consume event objects. Drain our private SDL queue
	// so axis/button events cannot accumulate throughout long game sessions.
	s.flush.Call(0, 0xffff)
	if !now.Before(s.nextDiscovery) {
		s.nextDiscovery = now.Add(250 * time.Millisecond)
		if err := s.refresh(); err != nil {
			s.failures++
			lgx.Warnf("SDL3 gamepad discovery failed attempt=%d err=%v", s.failures, err)
			if s.failures >= 3 {
				return xinputGamepad{}, 0, 0, err
			}
		} else {
			s.failures = 0
		}
	}
	var ids []uint32
	for id, handle := range s.gamepads {
		connected, _, _ := s.connected.Call(handle)
		if connected&0xff == 0 {
			s.close.Call(handle)
			delete(s.gamepads, id)
			s.nextDiscovery = time.Time{}
			lgx.Infof("SDL3 gamepad disconnected id=%d", id)
		} else {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	s.activeID = selectGamepadID(s.activeID, ids)
	if s.activeID == 0 {
		return xinputGamepad{}, 0, 0, nil
	}
	handle := s.gamepads[s.activeID]
	var buttons uint32
	for index := range sdlButtonMasks {
		pressed, _, _ := s.button.Call(handle, uintptr(index))
		if pressed&0xff != 0 {
			buttons |= 1 << index
		}
	}
	var axes [6]int16
	for index := range axes {
		value, _, _ := s.axis.Call(handle, uintptr(index))
		axes[index] = int16(value)
	}
	return normalizedSDLGamepad(buttons, axes), s.activeID, len(ids), nil
}

func (s *sdlBackend) refresh() error {
	s.logJoystickInventory()
	var count int32
	ptr, _, _ := s.getGamepads.Call(uintptr(unsafe.Pointer(&count)))
	if ptr == 0 {
		return fmt.Errorf("enumerate SDL3 gamepads: %s", s.errorText())
	}
	defer s.free.Call(ptr)
	if count < 0 || count > 256 {
		return fmt.Errorf("invalid SDL3 gamepad count %d", count)
	}
	ids := unsafe.Slice((*uint32)(unsafe.Pointer(ptr)), int(count))
	for _, id := range ids {
		if id == 0 || s.gamepads[id] != 0 {
			continue
		}
		handle, _, _ := s.open.Call(uintptr(id))
		if handle == 0 {
			lgx.Debugf("SDL3 gamepad open deferred id=%d err=%s", id, s.errorText())
			continue
		}
		s.gamepads[id] = handle
		ptr, _, _ := s.name.Call(handle)
		name := ""
		if ptr != 0 {
			name = windows.BytePtrToString((*byte)(unsafe.Pointer(ptr)))
		}
		lgx.Infof("SDL3 gamepad connected id=%d name=%q", id, name)
		if mapping, _, _ := s.mapping.Call(handle); mapping != 0 {
			lgx.Infof("SDL3 gamepad mapping id=%d mapping=%q", id, windows.BytePtrToString((*byte)(unsafe.Pointer(mapping))))
			s.free.Call(mapping)
		}
	}
	return nil
}

// Log each physical/virtual SDL joystick once, including unmapped devices that
// won't appear in SDL_GetGamepads. These IDs explain a native fallback without
// guessing a model's mapping from its name or changing working controllers.
func (s *sdlBackend) logJoystickInventory() {
	var count int32
	ptr, _, _ := s.getJoysticks.Call(uintptr(unsafe.Pointer(&count)))
	if ptr == 0 {
		lgx.Debugf("SDL3 joystick inventory unavailable: %s", s.errorText())
		return
	}
	defer s.free.Call(ptr)
	if count < 0 || count > 256 {
		lgx.Warnf("invalid SDL3 joystick inventory count=%d", count)
		return
	}
	next := make(map[uint32]bool, count)
	for _, id := range unsafe.Slice((*uint32)(unsafe.Pointer(ptr)), int(count)) {
		if id == 0 {
			continue
		}
		mapped, _, _ := s.isGamepad.Call(uintptr(id))
		isMapped := mapped&0xff != 0
		next[id] = isMapped
		if previous, seen := s.joysticks[id]; seen && previous == isMapped {
			continue
		}
		namePtr, _, _ := s.joystickName.Call(uintptr(id))
		name := ""
		if namePtr != 0 {
			name = windows.BytePtrToString((*byte)(unsafe.Pointer(namePtr)))
		}
		vendor, _, _ := s.vendor.Call(uintptr(id))
		product, _, _ := s.product.Call(uintptr(id))
		lgx.Infof("SDL3 joystick discovered id=%d name=%q vid=%04x pid=%04x mapped=%t", id, name, uint16(vendor), uint16(product), isMapped)
	}
	s.joysticks = next
}

func (s *sdlBackend) closeInput() {
	for id, handle := range s.gamepads {
		s.close.Call(handle)
		delete(s.gamepads, id)
	}
	if s.initialized {
		s.quit.Call()
		s.initialized = false
	}
	s.activeID = 0
	s.nextDiscovery = time.Time{}
	s.joysticks = make(map[uint32]bool)
	s.failures = 0
}

func (s *sdlBackend) shutdown() {
	s.closeInput()
	if s.dll != nil {
		_ = s.dll.Release()
		s.dll = nil
	}
}
