package controller

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	lgx "launcher/internal/logx"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

const (
	ControllerNative  = "native"
	ControllerVirtual = "virtual"
)

type ControllerInfo struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Connection       string `json:"connection"`
	Mode             string `json:"mode"`
	Known            bool   `json:"known"`
	Connected        bool   `json:"connected"`
	Active           bool   `json:"active"`
	NativeXInput     bool   `json:"nativeXInput"`
	BluetoothAddress string `json:"-"`
}

type controllerDevice struct {
	info     ControllerInfo
	path     string
	state    xinputGamepad
	instance uint32
}

type controllerInventory struct{ devices []controllerDevice }
type controllerPreference struct {
	Name             string `json:"name"`
	Connection       string `json:"connection"`
	Mode             string `json:"mode"`
	NativeXInput     bool   `json:"nativeXInput,omitempty"`
	BluetoothAddress string `json:"bluetoothAddress,omitempty"`
}
type controllerPreferences struct {
	Version     int                             `json:"version"`
	Controllers map[string]controllerPreference `json:"controllers"`
}
type controllerPreferenceStore struct {
	mu      sync.Mutex
	writeMu sync.Mutex
	path    string
	data    controllerPreferences
}

// Prefer a reported serial; otherwise Windows's interface path identifies the
// physical instance. Neither SDL's transient joystick ID nor XInput slot is a
// persistent identity for an unknown non-XInput pad.
func controllerIdentity(path, serial string, vendor, product uint16, connection string) string {
	identity := "path:" + strings.ToLower(path)
	if serial != "" {
		identity = "serial:" + serial
	}
	digest := sha256.Sum256([]byte(fmt.Sprintf("stagebox-controller-v1\n%04x:%04x\n%s\n%s", vendor, product, connection, identity)))
	return hex.EncodeToString(digest[:16])
}

func newControllerPreferenceStore() (*controllerPreferenceStore, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	return controllerPreferenceStoreAt(filepath.Join(filepath.Dir(exe), "assets", "controllers.json"))
}

func controllerPreferenceStoreAt(path string) (*controllerPreferenceStore, error) {
	store := &controllerPreferenceStore{path: path, data: controllerPreferences{Version: 1, Controllers: make(map[string]controllerPreference)}}
	file, err := os.Open(store.path)
	if os.IsNotExist(err) {
		lgx.Infof("controller preferences missing path=%q; non-XInput controllers will need a default", store.path)
		return store, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, 1024*1024+1))
	if err != nil {
		return nil, err
	}
	if len(data) > 1024*1024 {
		return nil, fmt.Errorf("controller preferences exceed 1 MiB")
	}
	decoder := json.NewDecoder(bytes.NewReader(bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&store.data); err != nil {
		return nil, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, fmt.Errorf("invalid trailing controller preferences")
	}
	if store.data.Version != 1 || store.data.Controllers == nil {
		return nil, fmt.Errorf("unsupported controller preference format")
	}
	for id, preference := range store.data.Controllers {
		if !validControllerID(id) || !validControllerMode(preference.Mode) || (preference.NativeXInput && preference.Mode != ControllerNative) || (preference.BluetoothAddress != "" && !validBluetoothAddress(preference.BluetoothAddress)) {
			return nil, fmt.Errorf("invalid saved controller preference")
		}
	}
	lgx.Infof("controller preferences loaded path=%q profiles=%d", store.path, len(store.data.Controllers))
	return store, nil
}

func validControllerID(id string) bool {
	decoded, err := hex.DecodeString(id)
	return err == nil && len(decoded) == 16 && id == strings.ToLower(id)
}
func validControllerMode(mode string) bool {
	return mode == ControllerNative || mode == ControllerVirtual
}

func (s *controllerPreferenceStore) apply(info ControllerInfo) ControllerInfo {
	info.Mode, info.Known = ControllerNative, info.NativeXInput
	if s == nil || info.NativeXInput {
		return info
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if saved, ok := s.data.Controllers[info.ID]; ok {
		info.Mode, info.Known = saved.Mode, true
		if info.BluetoothAddress == "" {
			info.BluetoothAddress = saved.BluetoothAddress
		}
	}
	return info
}

func (s *controllerPreferenceStore) save(info ControllerInfo, mode string) error {
	if s == nil {
		return fmt.Errorf("controller preferences are unavailable; check assets/controllers.json")
	}
	if !validControllerID(info.ID) || !validControllerMode(mode) || (info.BluetoothAddress != "" && !validBluetoothAddress(info.BluetoothAddress)) {
		return fmt.Errorf("invalid controller preference")
	}
	if info.NativeXInput && mode != ControllerNative {
		return fmt.Errorf("this controller already provides native Xbox input")
	}
	return s.edit(func(profiles map[string]controllerPreference) bool {
		address := info.BluetoothAddress
		if address == "" {
			address = profiles[info.ID].BluetoothAddress
		}
		profiles[info.ID] = controllerPreference{Name: info.Name, Connection: info.Connection, Mode: mode, NativeXInput: info.NativeXInput, BluetoothAddress: address}
		return true
	})
}

// All mutations use the same serialized atomic replacement. Never publish a
// changed in-memory map if writing the user's file failed.
func (s *controllerPreferenceStore) edit(change func(map[string]controllerPreference) bool) error {
	if s == nil {
		return fmt.Errorf("controller preferences are unavailable; check assets/controllers.json")
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	s.mu.Lock()
	updated := controllerPreferences{Version: 1, Controllers: make(map[string]controllerPreference, len(s.data.Controllers)+1)}
	for id, value := range s.data.Controllers {
		updated.Controllers[id] = value
	}
	s.mu.Unlock()
	if !change(updated.Controllers) {
		return nil
	}
	data, err := json.MarshalIndent(updated, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0755); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(s.path), ".controller-preferences-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	defer file.Close()
	if _, err := file.Write(append(data, '\n')); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(file.Name(), s.path); err != nil {
		return err
	}
	s.mu.Lock()
	s.data = updated
	s.mu.Unlock()
	return nil
}

func validBluetoothAddress(address string) bool {
	decoded, err := hex.DecodeString(address)
	return err == nil && len(decoded) == 6 && address == strings.ToLower(address) && address != "000000000000"
}

func (s *controllerPreferenceStore) rememberBluetooth(devices []controllerDevice) error {
	return s.edit(func(profiles map[string]controllerPreference) bool {
		changed := false
		for _, device := range devices {
			info := device.info
			if saved, exists := profiles[info.ID]; exists && validBluetoothAddress(info.BluetoothAddress) && (saved.BluetoothAddress != info.BluetoothAddress || saved.Connection != info.Connection) {
				saved.BluetoothAddress = info.BluetoothAddress
				saved.Connection = info.Connection
				profiles[info.ID] = saved
				changed = true
			}
		}
		return changed
	})
}

func (s *controllerPreferenceStore) remove(ids []string) error {
	for _, id := range ids {
		if !validControllerID(id) {
			return fmt.Errorf("invalid controller ID")
		}
	}
	return s.edit(func(profiles map[string]controllerPreference) bool {
		changed := false
		for _, id := range ids {
			if _, exists := profiles[id]; exists {
				delete(profiles, id)
				changed = true
			}
		}
		return changed
	})
}

func (s *controllerPreferenceStore) list(devices []controllerDevice) []ControllerInfo {
	result := make([]ControllerInfo, 0, len(devices))
	seen := make(map[string]bool)
	for _, device := range devices {
		result = append(result, s.apply(device.info))
		seen[device.info.ID] = true
	}
	if s != nil {
		s.mu.Lock()
		for id, saved := range s.data.Controllers {
			if !seen[id] {
				result = append(result, ControllerInfo{ID: id, Name: saved.Name, Connection: saved.Connection, Mode: saved.Mode, Known: true, NativeXInput: saved.NativeXInput, BluetoothAddress: saved.BluetoothAddress})
			}
		}
		s.mu.Unlock()
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].Connected != result[j].Connected {
			return result[i].Connected
		}
		if result[i].Active != result[j].Active {
			return result[i].Active
		}
		if result[i].Name != result[j].Name {
			return result[i].Name < result[j].Name
		}
		return result[i].ID < result[j].ID
	})
	return result
}

func controllerUsesVirtual(info ControllerInfo, gameEnabled bool) bool {
	return !info.NativeXInput && (info.Mode == ControllerVirtual || gameEnabled)
}

type ControllerState struct {
	ID                 string   `json:"id"`
	Connected          bool     `json:"connected"`
	Buttons            []string `json:"buttons"`
	LeftX              float64  `json:"leftX"`
	LeftY              float64  `json:"leftY"`
	RightX             float64  `json:"rightX"`
	RightY             float64  `json:"rightY"`
	LeftTrigger        float64  `json:"leftTrigger"`
	RightTrigger       float64  `json:"rightTrigger"`
	LeftTriggerSource  string   `json:"leftTriggerSource"`
	RightTriggerSource string   `json:"rightTriggerSource"`
	SDLLeftTrigger     int16    `json:"sdlLeftTrigger"`
	SDLRightTrigger    int16    `json:"sdlRightTrigger"`
	RawAxes            []int16  `json:"rawAxes"`
}

func diagnosticState(id string, gp xinputGamepad) ControllerState {
	state := ControllerState{ID: id, Connected: true, Buttons: []string{}, LeftX: float64(gp.sThumbLX) / 32768, LeftY: float64(gp.sThumbLY) / 32768, RightX: float64(gp.sThumbRX) / 32768, RightY: float64(gp.sThumbRY) / 32768, LeftTrigger: float64(gp.bLeftTrigger) / 255, RightTrigger: float64(gp.bRightTrigger) / 255}
	for _, button := range []struct {
		mask uint16
		name string
	}{
		{XINPUT_GAMEPAD_A, "A"}, {XINPUT_GAMEPAD_B, "B"}, {XINPUT_GAMEPAD_X, "X"}, {XINPUT_GAMEPAD_Y, "Y"},
		{XINPUT_GAMEPAD_LEFT_SHOULDER, "LB"}, {XINPUT_GAMEPAD_RIGHT_SHOULDER, "RB"},
		{XINPUT_GAMEPAD_BACK, "BACK"}, {XINPUT_GAMEPAD_START, "START"},
		{XINPUT_GAMEPAD_LEFT_THUMB, "L3"}, {XINPUT_GAMEPAD_RIGHT_THUMB, "R3"},
		{XINPUT_GAMEPAD_DPAD_UP, "DPAD_UP"}, {XINPUT_GAMEPAD_DPAD_DOWN, "DPAD_DOWN"}, {XINPUT_GAMEPAD_DPAD_LEFT, "DPAD_LEFT"}, {XINPUT_GAMEPAD_DPAD_RIGHT, "DPAD_RIGHT"},
	} {
		if gp.wButtons&button.mask != 0 {
			state.Buttons = append(state.Buttons, button.name)
		}
	}
	if gp.bLeftTrigger > 30 {
		state.Buttons = append(state.Buttons, "LT")
	}
	if gp.bRightTrigger > 30 {
		state.Buttons = append(state.Buttons, "RT")
	}
	return state
}
