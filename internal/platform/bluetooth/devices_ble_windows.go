//go:build windows

package bluetooth

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	lgx "launcher/internal/logx"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unicode/utf16"

	"golang.org/x/sys/windows"
)

//go:embed devices_ble.ps1
var bleDeviceScript string

type bluetoothOutput struct {
	bytes.Buffer
	limit int
}

func (b *bluetoothOutput) Write(data []byte) (int, error) {
	if b.Len()+len(data) > b.limit {
		return 0, fmt.Errorf("Windows Bluetooth output exceeded limit")
	}
	return b.Buffer.Write(data)
}

func runBLEOperation(action, endpoint, address string) ([]byte, error) {
	if action != "list" && action != "forget" {
		return nil, fmt.Errorf("invalid Bluetooth LE operation")
	}
	directory, err := windows.GetSystemDirectory()
	if err != nil {
		return nil, err
	}
	duration := 15 * time.Second
	if action == "forget" {
		duration = 45 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()
	encoded := utf16.Encode([]rune(bleDeviceScript))
	data := make([]byte, len(encoded)*2)
	for i, value := range encoded {
		binary.LittleEndian.PutUint16(data[i*2:], value)
	}
	command := exec.CommandContext(ctx, filepath.Join(directory, "WindowsPowerShell", "v1.0", "powershell.exe"), "-NoLogo", "-NoProfile", "-NonInteractive", "-Mta", "-EncodedCommand", base64.StdEncoding.EncodeToString(data))
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	for _, item := range os.Environ() {
		if !strings.HasPrefix(strings.ToUpper(item), "STAGEBOX_BT_") {
			command.Env = append(command.Env, item)
		}
	}
	command.Env = append(command.Env, "STAGEBOX_BT_ACTION="+action, "STAGEBOX_BT_DEVICE="+endpoint, "STAGEBOX_BT_ADDRESS="+address)
	output, diagnostic := &bluetoothOutput{limit: 1024 * 1024}, &bluetoothOutput{limit: 32 * 1024}
	command.Stdout, command.Stderr = output, diagnostic
	command.WaitDelay = time.Second
	if err := command.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("Windows Bluetooth LE %s timed out; refresh devices before retrying", action)
		}
		detail := strings.TrimSpace(diagnostic.String())
		if detail == "" {
			detail = err.Error()
		}
		return nil, fmt.Errorf("Windows Bluetooth LE %s failed: %s", action, detail)
	}
	for _, line := range strings.Split(strings.TrimSpace(diagnostic.String()), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			lgx.Debugf("Bluetooth LE %s: %s", action, line)
		}
	}
	return output.Bytes(), nil
}

func pairedBLEDevices() ([]PairedDevice, error) {
	data, err := runBLEOperation("list", "", "")
	if err != nil {
		return nil, err
	}
	return decodeBLEDevices(data)
}

func forgetBLEDevice(device PairedDevice) (PairedDevice, error) {
	if err := validatePairedDeviceID(device.ID); err != nil {
		return PairedDevice{}, err
	}
	if device.Transport != "LE" || device.deviceID == "" || device.ID != bleDeviceID(device.deviceID) {
		return PairedDevice{}, fmt.Errorf("invalid Bluetooth LE endpoint")
	}
	data, err := runBLEOperation("forget", device.deviceID, device.ControllerAddress())
	if err != nil {
		return PairedDevice{}, err
	}
	var result struct {
		Status  string `json:"status"`
		Address string `json:"address"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return PairedDevice{}, fmt.Errorf("read Bluetooth LE unpair result: %w", err)
	}
	if result.Status != "Unpaired" && result.Status != "AlreadyUnpaired" {
		return PairedDevice{}, fmt.Errorf("Windows did not confirm Bluetooth LE unpairing: %s", result.Status)
	}
	if result.Address != "" {
		if _, err := pairedDeviceAddress(result.Address); err != nil {
			return PairedDevice{}, fmt.Errorf("Bluetooth device was unpaired but its returned address was invalid: %w", err)
		}
		device.profileAddress = result.Address
	}
	return device, nil
}
