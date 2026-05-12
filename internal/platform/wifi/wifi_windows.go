//go:build windows

package wifi

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	lgx "launcher/internal/logx"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type Network struct {
	SSID           string `json:"ssid"`
	Authentication string `json:"authentication"`
	Encryption     string `json:"encryption"`
	Signal         int    `json:"signal"`
	Secure         bool   `json:"secure"`
	Known          bool   `json:"known"`
}

type Status struct {
	Available bool   `json:"available"`
	Connected bool   `json:"connected"`
	SSID      string `json:"ssid"`
	State     string `json:"state"`
}

func Scan() ([]Network, error) {
	return scanWithRetry(16 * time.Second)
}

func scanWithRetry(timeout time.Duration) ([]Network, error) {
	deadline := time.Now().Add(timeout)

	for {
		networks, err := scanOnce()
		if err == nil {
			return networks, nil
		}
		if !isTransientWLANError(err) || time.Now().After(deadline) {
			return nil, err
		}
		lgx.Debugf("WiFi scan waiting for WLAN service: %v", err)
		time.Sleep(2 * time.Second)
	}
}

func scanOnce() ([]Network, error) {
	out, err := netsh("wlan", "show", "networks", "mode=bssid").CombinedOutput()
	if err == nil {
		raw := string(out)
		lgx.Debugf("WiFi scan output: %q", strings.TrimSpace(raw))
		if scanSaysNoWirelessInterface(raw) {
			logWiFiDiagnostics()
			return nil, fmt.Errorf("no wireless interface detected by Windows WLAN service")
		}
		return markKnownNetworks(parseNetworks(raw)), nil
	}

	firstErr := wlanCommandError("netsh wlan show networks mode=bssid", err, out)
	lgx.Warnf("WiFi scan with BSSID failed: %v", firstErr)

	out, err = netsh("wlan", "show", "networks").CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%v; fallback failed: %w", firstErr, wlanCommandError("netsh wlan show networks", err, out))
	}

	lgx.Debugf("WiFi fallback scan output: %q", strings.TrimSpace(string(out)))
	if scanSaysNoWirelessInterface(string(out)) {
		logWiFiDiagnostics()
		return nil, fmt.Errorf("no wireless interface detected by Windows WLAN service")
	}
	return markKnownNetworks(parseNetworks(string(out))), nil
}

func Connect(ssid string, password string) error {
	ssid = strings.TrimSpace(ssid)
	if ssid == "" {
		return fmt.Errorf("ssid is required")
	}
	if err := waitForWLANReady(20 * time.Second); err != nil {
		return err
	}

	if strings.TrimSpace(password) == "" {
		if err := connectKnownProfile(ssid); err == nil {
			return nil
		} else {
			lgx.Warnf("WiFi saved profile connect failed ssid=%q err=%v", ssid, err)
			return fmt.Errorf("saved WiFi profile for %q did not connect; enter the password to refresh it: %w", ssid, err)
		}
	}

	if strings.TrimSpace(password) != "" {
		profileName := profileNameForSSID(ssid)
		profile, err := writeWPA2Profile(profileName, ssid, password)
		if err != nil {
			return err
		}
		defer os.Remove(profile)

		if err := connectWithProfile(ssid, profileName, profile, "current"); err == nil {
			return nil
		} else {
			lgx.Warnf("WiFi current-user StageBox profile connect failed ssid=%q profile=%q err=%v", ssid, profileName, err)
		}

		if err := connectWithProfile(ssid, profileName, profile, "all"); err == nil {
			return nil
		} else {
			lgx.Warnf("WiFi all-user StageBox profile connect failed ssid=%q profile=%q err=%v", ssid, profileName, err)
		}

		out, err := netsh("wlan", "connect", "name="+ssid, "ssid="+ssid).CombinedOutput()
		if err != nil {
			return fmt.Errorf("netsh wlan connect existing profile failed after StageBox profile fallback: %w: %s", err, strings.TrimSpace(string(out)))
		}
		lgx.Infof("WiFi connect requested ssid=%q using existing profile output=%q", ssid, strings.TrimSpace(string(out)))
		return waitForConnection(ssid, 40*time.Second)
	}

	return connectKnownProfile(ssid)
}

func connectWithProfile(ssid string, profileName string, profilePath string, userScope string) error {
	if out, err := netsh("wlan", "disconnect").CombinedOutput(); err != nil {
		lgx.Debugf("WiFi disconnect skipped err=%v output=%q", err, strings.TrimSpace(string(out)))
	}
	if out, err := netsh("wlan", "delete", "profile", "name="+profileName).CombinedOutput(); err != nil {
		lgx.Debugf("WiFi profile delete skipped profile=%q err=%v output=%q", profileName, err, strings.TrimSpace(string(out)))
	}
	out, err := netsh("wlan", "add", "profile", "filename="+profilePath, "user="+userScope).CombinedOutput()
	if err != nil {
		return fmt.Errorf("netsh wlan add profile user=%s failed: %w: %s", userScope, err, strings.TrimSpace(string(out)))
	}
	lgx.Infof("WiFi profile added ssid=%q profile=%q user=%s output=%q", ssid, profileName, userScope, strings.TrimSpace(string(out)))

	out, err = netsh("wlan", "connect", "name="+profileName, "ssid="+ssid).CombinedOutput()
	if err != nil {
		return fmt.Errorf("netsh wlan connect profile=%s failed: %w: %s", profileName, err, strings.TrimSpace(string(out)))
	}
	lgx.Infof("WiFi connect requested ssid=%q profile=%q user=%s output=%q", ssid, profileName, userScope, strings.TrimSpace(string(out)))
	return waitForConnection(ssid, 40*time.Second)
}

func connectKnownProfile(ssid string) error {
	var lastErr error
	for _, profileName := range []string{profileNameForSSID(ssid), ssid} {
		if profileName == "" {
			continue
		}
		out, err := netsh("wlan", "connect", "name="+profileName, "ssid="+ssid).CombinedOutput()
		if err != nil {
			lastErr = fmt.Errorf("netsh wlan connect profile=%s failed: %w: %s", profileName, err, strings.TrimSpace(string(out)))
			continue
		}
		lgx.Infof("WiFi connect requested ssid=%q profile=%q output=%q", ssid, profileName, strings.TrimSpace(string(out)))
		if err := waitForConnection(ssid, 40*time.Second); err != nil {
			lastErr = err
			continue
		}
		return nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no saved profile found")
	}
	return lastErr
}

func CurrentStatus() Status {
	out, err := netsh("wlan", "show", "interfaces").CombinedOutput()
	if err != nil {
		text := strings.TrimSpace(string(out))
		if wlanServiceNotRunning(text) {
			return Status{State: "service stopped"}
		}
		lgx.Warnf("WiFi status query failed: %v: %s", err, text)
		return Status{State: "unknown"}
	}
	lgx.Debugf("WiFi status output: %q", strings.TrimSpace(string(out)))
	return parseStatus(string(out))
}

func netsh(args ...string) *exec.Cmd {
	return hiddenCommand(windowsToolPath("netsh.exe"), args...)
}

func sc(args ...string) *exec.Cmd {
	return hiddenCommand(windowsToolPath("sc.exe"), args...)
}

func hiddenCommand(name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}
	return cmd
}

func scanSaysNoWirelessInterface(raw string) bool {
	text := strings.ToLower(raw)
	if strings.Contains(text, "no wireless interface") {
		return true
	}
	return strings.Contains(text, "no hay") && strings.Contains(text, "interfaz") && strings.Contains(text, "inal")
}

func wlanServiceNotRunning(raw string) bool {
	text := strings.ToLower(raw)
	return strings.Contains(text, "wlansvc") && strings.Contains(text, "not running")
}

func wlanCommandError(command string, err error, out []byte) error {
	text := strings.TrimSpace(string(out))
	if text == "" {
		return fmt.Errorf("%s failed: %w", command, err)
	}
	return fmt.Errorf("%s failed: %w: %s", command, err, text)
}

func isTransientWLANError(err error) bool {
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "wlansvc") && strings.Contains(text, "not running")
}

func waitForWLANReady(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var last Status

	for {
		last = CurrentStatus()
		if last.Available {
			return nil
		}
		if last.State != "service stopped" || time.Now().After(deadline) {
			break
		}
		lgx.Debug("WiFi connect waiting for WLAN service")
		time.Sleep(2 * time.Second)
	}

	if last.State == "service stopped" {
		return fmt.Errorf("WlanSvc is not running; wait/retry or rerun the SYSTEM startup Wi-Fi task")
	}
	return fmt.Errorf("Wi-Fi interface is not available; state=%s", last.State)
}

func waitForConnection(ssid string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var last Status

	for {
		last = CurrentStatus()
		if last.Connected && strings.EqualFold(last.SSID, ssid) {
			lgx.Infof("WiFi connected ssid=%q", ssid)
			return nil
		}
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(1 * time.Second)
	}

	if last.Connected && last.SSID != "" {
		logWiFiDiagnostics()
		return fmt.Errorf("WiFi connection did not switch to %q; still connected to %q", ssid, last.SSID)
	}
	if last.State == "service stopped" {
		logWiFiDiagnostics()
		return fmt.Errorf("WiFi connection to %q was requested but WlanSvc did not stay running long enough to verify it", ssid)
	}
	if !last.Available {
		logWiFiDiagnostics()
		return fmt.Errorf("WiFi connection to %q was requested but no wireless interface is available; state=%s", ssid, last.State)
	}
	logWiFiDiagnostics()
	return fmt.Errorf("WiFi connection to %q was requested but did not complete; state=%s", ssid, last.State)
}

func parseStatus(raw string) Status {
	status := Status{Available: !scanSaysNoWirelessInterface(raw)}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)

		switch key {
		case "state", "estado":
			status.State = value
			status.Connected = isConnectedState(value)
		case "ssid":
			if status.SSID == "" {
				status.SSID = value
			}
		}
	}
	return status
}

func isConnectedState(state string) bool {
	state = strings.ToLower(strings.TrimSpace(state))
	return state == "connected" || state == "conectado"
}

func logWiFiDiagnostics() {
	for _, args := range [][]string{
		{"wlan", "show", "interfaces"},
		{"wlan", "show", "drivers"},
		{"wlan", "show", "profiles"},
	} {
		out, err := netsh(args...).CombinedOutput()
		name := "netsh " + strings.Join(args, " ")
		if err != nil {
			lgx.Warnf("WiFi diagnostic failed %s: %v: %s", name, err, strings.TrimSpace(string(out)))
			continue
		}
		lgx.Warnf("WiFi diagnostic %s: %q", name, strings.TrimSpace(string(out)))
	}
}

func windowsToolPath(name string) string {
	windir := os.Getenv("WINDIR")
	if windir == "" {
		windir = os.Getenv("SystemRoot")
	}
	if windir != "" {
		candidates := []string{
			filepath.Join(windir, "System32", name),
			filepath.Join(windir, "Sysnative", name),
		}
		for _, candidate := range candidates {
			if _, err := os.Stat(candidate); err == nil {
				return candidate
			}
		}
	}
	return name
}

func parseNetworks(raw string) []Network {
	var networks []Network
	seen := map[string]int{}
	var cur *Network

	flush := func() {
		if cur == nil || cur.SSID == "" {
			return
		}
		cur.Secure = !isOpenAuth(cur.Authentication) && cur.Authentication != ""
		if idx, ok := seen[cur.SSID]; ok {
			if cur.Signal > networks[idx].Signal {
				networks[idx].Signal = cur.Signal
			}
			if networks[idx].Authentication == "" {
				networks[idx].Authentication = cur.Authentication
			}
			if networks[idx].Encryption == "" {
				networks[idx].Encryption = cur.Encryption
			}
			networks[idx].Secure = networks[idx].Secure || cur.Secure
			return
		}
		seen[cur.SSID] = len(networks)
		networks = append(networks, *cur)
	}

	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)

		if strings.HasPrefix(key, "SSID ") {
			flush()
			cur = &Network{SSID: value}
			continue
		}
		if cur == nil {
			continue
		}

		switch strings.ToLower(key) {
		case "authentication", "autenticación":
			cur.Authentication = value
		case "encryption", "cifrado":
			cur.Encryption = value
		case "signal", "señal":
			value = strings.TrimSuffix(value, "%")
			if signal, err := strconv.Atoi(strings.TrimSpace(value)); err == nil && signal > cur.Signal {
				cur.Signal = signal
			}
		}
	}
	flush()

	return networks
}

func markKnownNetworks(networks []Network) []Network {
	profiles := savedProfileNames()
	if len(profiles) == 0 {
		return networks
	}
	for i := range networks {
		ssid := strings.ToLower(networks[i].SSID)
		stageboxProfile := strings.ToLower(profileNameForSSID(networks[i].SSID))
		networks[i].Known = profiles[ssid] || profiles[stageboxProfile]
	}
	return networks
}

func savedProfileNames() map[string]bool {
	out, err := netsh("wlan", "show", "profiles").CombinedOutput()
	if err != nil {
		lgx.Debugf("WiFi saved profile query failed: %v: %s", err, strings.TrimSpace(string(out)))
		return nil
	}

	profiles := map[string]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		if !strings.Contains(key, "profile") && !strings.Contains(key, "perfil") {
			continue
		}
		value = strings.TrimSpace(value)
		if value != "" {
			profiles[strings.ToLower(value)] = true
		}
	}
	return profiles
}

func isOpenAuth(auth string) bool {
	auth = strings.ToLower(strings.TrimSpace(auth))
	return auth == "open" || auth == "abierta" || auth == "abrir"
}

func profileNameForSSID(ssid string) string {
	sum := sha1.Sum([]byte(ssid))
	return "StageBox-" + hex.EncodeToString(sum[:])[:12]
}

func writeWPA2Profile(profileName string, ssid string, password string) (string, error) {
	var profileEsc bytes.Buffer
	var ssidEsc bytes.Buffer
	var passwordEsc bytes.Buffer
	if err := xml.EscapeText(&profileEsc, []byte(profileName)); err != nil {
		return "", err
	}
	if err := xml.EscapeText(&ssidEsc, []byte(ssid)); err != nil {
		return "", err
	}
	if err := xml.EscapeText(&passwordEsc, []byte(password)); err != nil {
		return "", err
	}

	profile := fmt.Sprintf(`<?xml version="1.0"?>
<WLANProfile xmlns="http://www.microsoft.com/networking/WLAN/profile/v1">
  <name>%s</name>
  <SSIDConfig>
    <SSID>
      <name>%s</name>
    </SSID>
  </SSIDConfig>
  <connectionType>ESS</connectionType>
  <connectionMode>auto</connectionMode>
  <MSM>
    <security>
      <authEncryption>
        <authentication>WPA2PSK</authentication>
        <encryption>AES</encryption>
        <useOneX>false</useOneX>
      </authEncryption>
      <sharedKey>
        <keyType>passPhrase</keyType>
        <protected>false</protected>
        <keyMaterial>%s</keyMaterial>
      </sharedKey>
    </security>
  </MSM>
</WLANProfile>`, profileEsc.String(), ssidEsc.String(), passwordEsc.String())

	f, err := os.CreateTemp("", "stagebox-wifi-*.xml")
	if err != nil {
		return "", err
	}
	defer f.Close()

	if _, err := f.WriteString(profile); err != nil {
		_ = os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}
