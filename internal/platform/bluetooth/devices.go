package bluetooth

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
)

type PairedDevice struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Address        string `json:"address"`
	Connected      bool   `json:"connected"`
	Transport      string `json:"transport"`
	deviceID       string
	profileAddress string
}

type PairedDeviceList struct {
	Devices  []PairedDevice `json:"devices"`
	Complete bool           `json:"complete"`
	Warning  string         `json:"warning"`
	Details  string         `json:"-"`
}

// The screen can display one working backend. Callers deciding whether a
// profile is still paired must reject incomplete snapshots instead.
func pairedDeviceList(classic []PairedDevice, classicErr error, le []PairedDevice, leErr error) PairedDeviceList {
	result := PairedDeviceList{Complete: classicErr == nil && leErr == nil}
	if classicErr != nil {
		classic = nil
	}
	if leErr != nil {
		le = nil
	}
	result.Devices = mergePairedDevices(classic, le)
	if result.Complete {
		return result
	}
	result.Details = errors.Join(classicErr, leErr).Error()
	switch {
	case classicErr != nil && leErr != nil:
		result.Warning = "Bluetooth devices could not be loaded. Try Refresh."
	case leErr != nil:
		result.Warning = "Bluetooth LE devices are unavailable. Showing Classic devices."
	default:
		result.Warning = "Classic Bluetooth devices are unavailable. Showing LE devices."
	}
	return result
}

func (result PairedDeviceList) verifiedDevices() ([]PairedDevice, error) {
	if !result.Complete {
		return nil, fmt.Errorf("cannot verify all Bluetooth pairings: %s", result.Details)
	}
	for _, device := range result.Devices {
		if device.ControllerAddress() == "" {
			return nil, fmt.Errorf("a paired Bluetooth LE device has no reported address; use Bluetooth Devices to forget it before removing saved profiles")
		}
	}
	return result.Devices, nil
}

// A paired LE endpoint can omit its address while offline. Its Windows ID,
// rather than optional metadata, is the stable identity used for selection.
func bleDeviceID(endpoint string) string {
	digest := sha256.Sum256([]byte(endpoint))
	return "le:" + hex.EncodeToString(digest[:16])
}

func validatePairedDeviceID(id string) error {
	if len(id) == 12 {
		_, err := pairedDeviceAddress(id)
		return err
	}
	if len(id) != 35 || !strings.HasPrefix(id, "le:") {
		return fmt.Errorf("invalid Bluetooth device ID")
	}
	if _, err := hex.DecodeString(id[3:]); err != nil || id[3:] != strings.ToLower(id[3:]) {
		return fmt.Errorf("invalid Bluetooth LE endpoint ID")
	}
	return nil
}

func (device PairedDevice) ControllerAddress() string {
	if len(device.ID) == 12 {
		if _, err := pairedDeviceAddress(device.ID); err == nil {
			return device.ID
		}
	}
	return device.profileAddress
}

func displayAddress(address string) string {
	parts := make([]string, 6)
	for i := range parts {
		parts[i] = strings.ToUpper(address[i*2 : i*2+2])
	}
	return strings.Join(parts, ":")
}

func mergePairedDevices(classic, le []PairedDevice) []PairedDevice {
	result := make([]PairedDevice, 0, len(classic)+len(le))
	seen := map[string]int{}
	for _, group := range [][]PairedDevice{classic, le} {
		for _, device := range group {
			if at, ok := seen[device.ID]; ok {
				result[at].Connected = result[at].Connected || device.Connected
				continue
			}
			seen[device.ID] = len(result)
			result = append(result, device)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Name != result[j].Name {
			return result[i].Name < result[j].Name
		}
		return result[i].ID < result[j].ID
	})
	return result
}

func decodeBLEDevices(data []byte) ([]PairedDevice, error) {
	if len(data) > 1024*1024 {
		return nil, fmt.Errorf("Bluetooth LE list exceeds 1 MiB")
	}
	var raw struct {
		Devices []struct {
			DeviceID  string `json:"deviceID"`
			Address   string `json:"address"`
			Name      string `json:"name"`
			Connected bool   `json:"connected"`
		} `json:"devices"`
	}
	decoder := json.NewDecoder(bytes.NewReader(bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&raw); err != nil {
		return nil, fmt.Errorf("read Windows Bluetooth LE list: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, fmt.Errorf("invalid trailing Bluetooth LE data")
	}
	if raw.Devices == nil || len(raw.Devices) > 1024 {
		return nil, fmt.Errorf("invalid Windows Bluetooth LE list")
	}
	devices := make([]PairedDevice, 0, len(raw.Devices))
	for _, item := range raw.Devices {
		if item.Address != "" {
			if _, err := pairedDeviceAddress(item.Address); err != nil {
				return nil, err
			}
		}
		if item.DeviceID == "" || len(item.DeviceID) > 8192 || strings.ContainsRune(item.DeviceID, '\x00') {
			return nil, fmt.Errorf("invalid Windows Bluetooth LE endpoint")
		}
		name := strings.TrimSpace(item.Name)
		if name == "" {
			name = "Bluetooth LE device"
		}
		address := ""
		if item.Address != "" {
			address = displayAddress(item.Address)
		}
		devices = append(devices, PairedDevice{ID: bleDeviceID(item.DeviceID), Name: name, Address: address, Connected: item.Connected, Transport: "LE", deviceID: item.DeviceID, profileAddress: item.Address})
	}
	return mergePairedDevices(nil, devices), nil
}

func pairedDeviceAddress(id string) (uint64, error) {
	if len(id) != 12 {
		return 0, fmt.Errorf("invalid Bluetooth device ID")
	}
	for _, c := range id {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return 0, fmt.Errorf("invalid Bluetooth device ID")
		}
	}
	address, err := strconv.ParseUint(id, 16, 48)
	if err != nil || address == 0 {
		return 0, fmt.Errorf("invalid Bluetooth device ID")
	}
	return address, nil
}
