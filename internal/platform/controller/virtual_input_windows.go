//go:build windows

package controller

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	lgx "launcher/internal/logx"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

//go:embed hidmaestro_manifest.json
var hidMaestroManifest []byte

func virtualHelperPath() (string, error) {
	if runtime.GOARCH != "amd64" {
		return "", fmt.Errorf("virtual Xbox input currently requires Windows x64")
	}
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(exe), "assets", "hidmaestro", "StageBox.HIDMaestro.exe"), nil
}

func VirtualInputAvailability() (bool, string) {
	path, err := virtualHelperPath()
	if err == nil {
		_, err = os.Stat(path)
	}
	if err != nil {
		return false, "Copy the bundled assets/hidmaestro folder beside StageBox to use virtual Xbox input."
	}
	return true, "The physical controller is hidden from the game to prevent double input. Install HidHide during StageBox setup; Windows asks for administrator permission to create the virtual controller."
}

func verifyVirtualHelper(path string) error {
	var files map[string]string
	if err := json.Unmarshal(hidMaestroManifest, &files); err != nil {
		return err
	}
	if files["StageBox.HIDMaestro.exe"] == "" {
		return fmt.Errorf("virtual controller bundle has no integrity manifest")
	}
	for name, want := range files {
		if filepath.Base(name) != name {
			return fmt.Errorf("invalid virtual controller bundle manifest")
		}
		file, err := os.Open(filepath.Join(filepath.Dir(path), name))
		if err != nil {
			return fmt.Errorf("virtual controller bundle is incomplete: %s", name)
		}
		hash := sha256.New()
		_, err = io.Copy(hash, file)
		file.Close()
		if err != nil || hex.EncodeToString(hash.Sum(nil)) != want {
			return fmt.Errorf("virtual controller file changed: %s; restore the bundled assets/hidmaestro folder", name)
		}
	}
	return nil
}

func (b *ControllerBridge) EnableVirtualInput(ctx context.Context, gameEnabled bool) error {
	routes := b.virtualRoutes(gameEnabled)
	if len(routes) == 0 {
		return nil
	}
	if len(routes) > 4 {
		return fmt.Errorf("Windows supports at most four virtual Xbox controllers; change controller defaults before launching")
	}
	paths := make([]string, len(routes))
	for i, route := range routes {
		if route.path == "" {
			return fmt.Errorf("controller %q has no Windows HID path", route.info.Name)
		}
		paths[i] = route.path
	}
	session := newVirtualInputSession(b.ctx, paths[0], paths[1:]...)
	if !b.virtualInput.CompareAndSwap(nil, session) {
		session.cancel()
		return fmt.Errorf("a virtual input session is already active")
	}
	success := false
	defer func() {
		if !success {
			session.close()
			b.virtualInput.CompareAndSwap(session, nil)
			b.resetInput.Store(true)
		}
	}()
	startup, cancel := context.WithTimeout(session.ctx, 90*time.Second)
	defer cancel()
	stopCaller := context.AfterFunc(ctx, cancel)
	defer stopCaller()
	select {
	case <-session.pinned:
	case <-startup.Done():
		return startup.Err()
	case <-b.ctx.Done():
		return b.ctx.Err()
	}
	conn, closeProcess, err := startVirtualHelper(startup, paths)
	if err != nil {
		return err
	}
	if err := session.attach(conn, closeProcess); err != nil {
		return err
	}
	success = true
	lgx.Infof("virtual Xbox input enabled physicalPaths=%q count=%d backend=HIDMaestro", paths, len(paths))
	return nil
}

func (b *ControllerBridge) DisableVirtualInput() error {
	session := b.virtualInput.Load()
	if session == nil {
		return nil
	}
	started := time.Now()
	err := session.close()
	b.virtualInput.CompareAndSwap(session, nil)
	b.resetInput.Store(true)
	lgx.Infof("virtual Xbox input session closed elapsed=%s err=%v", time.Since(started).Round(time.Millisecond), err)
	return err
}

type virtualHello struct {
	FeedbackVersion int            `json:"feedbackVersion"`
	Type            string         `json:"type"`
	Token           string         `json:"token"`
	Error           string         `json:"error"`
	Marker          string         `json:"marker"`
	Index           int            `json:"xinputIndex"`
	Hidden          bool           `json:"physicalHidden"`
	Device          string         `json:"physicalInstance"`
	Controllers     []virtualHello `json:"controllers"`
}

func startVirtualHelper(ctx context.Context, sourcePaths []string) (net.Conn, func() error, error) {
	path, err := virtualHelperPath()
	if err != nil {
		return nil, nil, err
	}
	if err := verifyVirtualHelper(path); err != nil {
		return nil, nil, err
	}
	listener, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		return nil, nil, err
	}
	defer listener.Close()
	stop := context.AfterFunc(ctx, func() { listener.Close() })
	defer stop()
	var nonce [32]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, nil, err
	}
	token := hex.EncodeToString(nonce[:])
	var created, exited, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(windows.CurrentProcess(), &created, &exited, &kernel, &user); err != nil {
		return nil, nil, err
	}
	creation := uint64(created.HighDateTime)<<32 | uint64(created.LowDateTime)
	args := fmt.Sprintf("%d %s %d %d", listener.Addr().(*net.TCPAddr).Port, token, os.Getpid(), creation)
	process, err := runElevatedHelper(ctx, path, args)
	if err != nil {
		return nil, nil, err
	}
	closeProcess := func() error {
		defer windows.CloseHandle(process)
		result, err := windows.WaitForSingleObject(process, 15000)
		if err != nil {
			return err
		}
		if result == uint32(windows.WAIT_TIMEOUT) {
			return fmt.Errorf("virtual controller helper did not finish device cleanup within 15 seconds")
		}
		var code uint32
		if err := windows.GetExitCodeProcess(process, &code); err != nil {
			return err
		}
		if code != 0 {
			return fmt.Errorf("virtual controller helper exited with code %d", code)
		}
		return nil
	}
	ready := false
	defer func() {
		if !ready {
			closeProcess()
		}
	}()
	for {
		conn, err := listener.AcceptTCP()
		if err != nil {
			return nil, nil, fmt.Errorf("virtual controller startup interrupted: %w", err)
		}
		conn.SetNoDelay(true)
		deadline, _ := ctx.Deadline()
		helloDeadline := time.Now().Add(5 * time.Second)
		if !deadline.IsZero() && deadline.Before(helloDeadline) {
			helloDeadline = deadline
		}
		conn.SetReadDeadline(helloDeadline)
		stopConnection := context.AfterFunc(ctx, func() { conn.Close() })
		closeConnection := func() {
			stopConnection()
			conn.Close()
		}
		reader := bufio.NewReaderSize(conn, 32768)
		line, err := reader.ReadSlice('\n')
		var hello virtualHello
		if err != nil || json.Unmarshal(line, &hello) != nil || hello.Type != "hello" || subtle.ConstantTimeCompare([]byte(hello.Token), []byte(token)) != 1 {
			closeConnection()
			continue
		}
		conn.SetReadDeadline(deadline)
		conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		request, _ := json.Marshal(map[string]any{"type": "start", "physicalPaths": sourcePaths, "feedbackVersion": virtualFeedbackVersion})
		if _, err := conn.Write(append(request, '\n')); err != nil {
			closeConnection()
			return nil, nil, err
		}
		line, err = reader.ReadSlice('\n')
		if err != nil {
			closeConnection()
			return nil, nil, fmt.Errorf("virtual controller setup failed: %w", err)
		}
		if err := json.Unmarshal(line, &hello); err != nil {
			closeConnection()
			return nil, nil, err
		}
		if hello.Type != "ready" {
			closeConnection()
			return nil, nil, fmt.Errorf("virtual controller setup failed: %s", hello.Error)
		}
		if hello.FeedbackVersion != virtualFeedbackVersion {
			closeConnection()
			return nil, nil, fmt.Errorf("virtual controller feedback version mismatch; copy the matching assets/hidmaestro bundle")
		}
		if len(hello.Controllers) != len(sourcePaths) {
			closeConnection()
			return nil, nil, fmt.Errorf("invalid virtual controller identity")
		}
		seenSlots := make(map[int]bool)
		for i, controller := range hello.Controllers {
			if controller.Index < 0 || controller.Index > 3 || seenSlots[controller.Index] || !strings.HasPrefix(controller.Marker, "HM_") || !controller.Hidden || !strings.HasPrefix(strings.ToUpper(controller.Device), "HID\\") {
				closeConnection()
				return nil, nil, fmt.Errorf("invalid virtual controller identity")
			}
			seenSlots[controller.Index] = true
			// Check a fresh open as the allowlisted feeder; held SDL handles alone
			// cannot prove that a later reconnect will remain readable.
			physicalPath, err := windows.UTF16PtrFromString(sourcePaths[i])
			if err != nil {
				closeConnection()
				return nil, nil, err
			}
			physical, err := windows.CreateFile(physicalPath, 0, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
			if err != nil {
				closeConnection()
				return nil, nil, fmt.Errorf("StageBox cannot read the hidden physical controller; reconnect it and retry: %w", err)
			}
			windows.CloseHandle(physical)
			lgx.Infof("HIDMaestro virtual controller ready marker=%s xinputIndex=%d physicalInstance=%q", controller.Marker, controller.Index, controller.Device)
		}
		conn.SetReadDeadline(time.Time{})
		conn.SetWriteDeadline(time.Time{})
		stopConnection()
		ready = true
		return &virtualFeedbackConnection{Conn: conn, reader: reader}, closeProcess, nil
	}
}

// Recover only our stale journal before SDL opens controllers. The public
// HidHide configuration API needs no elevation, so this path has no UAC.
// Nothing runs on ordinary startups without a leftover session journal.
func recoverVirtualInputState(ctx context.Context) {
	path, err := virtualHelperPath()
	if err != nil {
		return
	}
	journal := filepath.Join(filepath.Dir(filepath.Dir(path)), "hidhide-session.json")
	if _, err := os.Stat(journal); os.IsNotExist(err) {
		return
	}
	if err := verifyVirtualHelper(path); err != nil {
		lgx.Warnf("HidHide recovery bundle: %v", err)
		return
	}
	recovery, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(recovery, path, "--recover-hidhide")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	output, err := cmd.CombinedOutput()
	if err != nil {
		lgx.Warnf("HidHide recovery failed: %v %s", err, strings.TrimSpace(string(output)))
		return
	}
	lgx.Info("HidHide stale session settings restored")
}

// ShellExecuteEx owns elevation, with no elevated launcher and no generic
// executable/command parameters exposed to the frontend.
type helperShellExecute struct {
	Size, Mask                        uint32
	Window                            uintptr
	Verb, File, Parameters, Directory *uint16
	Show                              int32
	Instance                          uintptr
	IDList                            uintptr
	Class                             *uint16
	ClassKey                          uintptr
	HotKey                            uint32
	Icon                              uintptr
	Process                           windows.Handle
}

func runElevatedHelper(ctx context.Context, path, args string) (windows.Handle, error) {
	verb, _ := windows.UTF16PtrFromString("runas")
	file, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	parameters, _ := windows.UTF16PtrFromString(args)
	directory, _ := windows.UTF16PtrFromString(filepath.Dir(path))
	info := helperShellExecute{Mask: 0x40 | 0x100, Verb: verb, File: file, Parameters: parameters, Directory: directory, Show: 0}
	info.Size = uint32(unsafe.Sizeof(info))
	proc := windows.NewLazySystemDLL("shell32.dll").NewProc("ShellExecuteExW")
	type launched struct {
		handle windows.Handle
		err    error
	}
	result := make(chan launched, 1)
	go func() {
		value, _, err := proc.Call(uintptr(unsafe.Pointer(&info)))
		runtime.KeepAlive(verb)
		runtime.KeepAlive(file)
		runtime.KeepAlive(parameters)
		runtime.KeepAlive(directory)
		if value == 0 {
			result <- launched{err: fmt.Errorf("Windows virtual-controller elevation was cancelled or failed: %w", err)}
			return
		}
		if info.Process == 0 {
			result <- launched{err: fmt.Errorf("Windows did not return a virtual-controller helper process handle")}
			return
		}
		result <- launched{handle: info.Process}
	}()
	select {
	case launched := <-result:
		return launched.handle, launched.err
	case <-ctx.Done():
		go func() {
			launched := <-result
			if launched.handle != 0 {
				windows.CloseHandle(launched.handle)
			}
		}()
		return 0, ctx.Err()
	}
}
