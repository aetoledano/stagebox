//go:build windows

package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

const testControllerMapping = "03000000c82d00001230000000000000,8BitDo Ultimate,a:b0,back:b10,start:b11,platform:Windows,\n"
const testXInputControllerMapping = "xinput,XInput Controller,a:b0,back:b6,start:b7,platform:Windows,\n"

func TestControllerDatabaseLoadsBaseBeforeOverrides(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	paths := controllerDatabasePathsIn(dir)
	if len(paths) != 3 || paths[0] != filepath.Join(dir, "assets", "gamecontrollerdb.txt") || paths[2] != filepath.Join(dir, "assets", "gamecontrollerdb-overrides.txt") {
		t.Fatalf("incorrect base/override paths: %v", paths)
	}
	for i, path := range paths {
		data := strings.Replace(testControllerMapping, "start:b11", []string{"start:b8", "start:b9", "start:b10"}[i], 1)
		if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	files, _, err := readControllerMappingFiles(paths)
	if err != nil || len(files) != 3 {
		t.Fatalf("read mapping stack: files=%d err=%v", len(files), err)
	}
	if files[0].source != "database" || files[2].source != "overrides" || !strings.Contains(string(files[2].data), "start:b10") {
		t.Fatalf("base/overrides loaded in wrong order: %v", files)
	}
}

func TestControllerDatabaseRefreshesBaseAndPreservesOverrides(t *testing.T) {
	var requests atomic.Int32
	initial := testControllerMapping + testXInputControllerMapping
	updated := strings.Replace(initial, "start:b11", "start:b12", 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) == 1 {
			w.Write([]byte(initial))
		} else {
			w.Write([]byte(updated))
		}
	}))
	defer server.Close()
	path := filepath.Join(t.TempDir(), "assets", "gamecontrollerdb.txt")
	if err := downloadControllerDatabase(context.Background(), path, server.URL, server.Client()); err != nil {
		t.Fatal(err)
	}
	manual := strings.Replace(testControllerMapping, "start:b11", "start:b7", 1)
	overrides := filepath.Join(filepath.Dir(path), "gamecontrollerdb-overrides.txt")
	if err := os.WriteFile(overrides, []byte(manual), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := downloadControllerDatabase(context.Background(), path, server.URL, server.Client()); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != updated || requests.Load() != 2 {
		t.Fatalf("base database was not refreshed: data=%q requests=%d err=%v", data, requests.Load(), err)
	}
	data, err = os.ReadFile(overrides)
	if err != nil || string(data) != manual {
		t.Fatalf("manual overrides changed: data=%q err=%v", data, err)
	}
	data, err = os.ReadFile(filepath.Join(filepath.Dir(path), "gamecontrollerdb-before-refresh.txt"))
	if err != nil || string(data) != initial {
		t.Fatalf("original base not backed up: data=%q err=%v", data, err)
	}
}

func TestControllerDatabaseRefreshPreservesConcurrentOverrides(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gamecontrollerdb.txt")
	overrides := filepath.Join(filepath.Dir(path), "gamecontrollerdb-overrides.txt")
	manual := "# manually created during request\n" + testControllerMapping
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := os.WriteFile(overrides, []byte(manual), 0o644); err != nil {
			t.Error(err)
		}
		w.Write([]byte(testControllerMapping))
	}))
	defer server.Close()
	if err := downloadControllerDatabase(context.Background(), path, server.URL, server.Client()); err != nil {
		t.Fatal(err)
	}
	if err := publishControllerMappingFile(overrides, []byte("# empty defaults\n"), false); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(overrides)
	if err != nil || string(data) != manual {
		t.Fatalf("concurrent manual file was replaced: data=%q err=%v", data, err)
	}
}

func TestControllerDatabaseRejectsFailedDownloads(t *testing.T) {
	for _, sample := range []struct {
		name   string
		status int
		body   string
	}{
		{"http-error", http.StatusServiceUnavailable, testControllerMapping},
		{"html", http.StatusOK, "<html>not a database</html>"},
		{"empty", http.StatusOK, "# no mappings\n"},
		{"wrong-platform", http.StatusOK, strings.Replace(testControllerMapping, "Windows", "Linux", 1)},
		{"too-large", http.StatusOK, strings.Repeat("#", controllerDatabaseLimit+1)},
	} {
		t.Run(sample.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(sample.status)
				w.Write([]byte(sample.body))
			}))
			defer server.Close()
			path := filepath.Join(t.TempDir(), "gamecontrollerdb.txt")
			if err := os.WriteFile(path, []byte(testControllerMapping), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := downloadControllerDatabase(context.Background(), path, server.URL, server.Client()); err == nil {
				t.Fatal("bad download accepted")
			}
			if data, err := os.ReadFile(path); err != nil || string(data) != testControllerMapping {
				t.Fatalf("bad download replaced cached base: data=%q err=%v", data, err)
			}
			if files, _ := filepath.Glob(filepath.Join(filepath.Dir(path), ".gamecontrollerdb-*.tmp")); len(files) != 0 {
				t.Fatalf("temporary files leaked: %v", files)
			}
		})
	}
}

func TestControllerMappingFilesDetectContentChangesAndRemoval(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gamecontrollerdb.txt")
	paths := []string{path}
	_, absent, err := readControllerMappingFiles(paths)
	if err != nil {
		t.Fatal(err)
	}
	writeMapping := func(data string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeMapping("\ufeff" + testControllerMapping)
	files, initial, err := readControllerMappingFiles(paths)
	if err != nil || len(files) != 1 || initial == absent {
		t.Fatalf("new BOM-prefixed file not detected: files=%v err=%v", files, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	writeMapping("\ufeff" + strings.Replace(testControllerMapping, "start:b11", "start:b12", 1))
	if err := os.Chtimes(path, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	_, changed, err := readControllerMappingFiles(paths)
	if err != nil || changed == initial {
		t.Fatalf("same-size/timestamp content edit not detected: %v", err)
	}
	writeMapping("broken partial save")
	if _, _, err := readControllerMappingFiles(paths); err == nil {
		t.Fatal("malformed manual edit accepted")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	_, removed, err := readControllerMappingFiles(paths)
	if err != nil || removed != absent {
		t.Fatalf("file removal not detected: %v", err)
	}
}
