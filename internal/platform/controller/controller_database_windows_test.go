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

func TestControllerDatabaseDownloadPreservesManualEdits(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Write([]byte(testControllerMapping + testXInputControllerMapping))
	}))
	defer server.Close()
	path := filepath.Join(t.TempDir(), "assets", "gamecontrollerdb.txt")
	if err := downloadControllerDatabase(context.Background(), path, server.URL, server.Client()); err != nil {
		t.Fatal(err)
	}
	manual := strings.Replace(testControllerMapping, "start:b11", "start:b7", 1)
	if err := os.WriteFile(path, []byte(manual), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := downloadControllerDatabase(context.Background(), path, server.URL, server.Client()); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != manual || requests.Load() != 1 {
		t.Fatalf("manual edits were not preserved: data=%q requests=%d err=%v", data, requests.Load(), err)
	}
}

func TestControllerDatabaseDownloadDoesNotReplaceConcurrentFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gamecontrollerdb.txt")
	manual := "# manually created during request\n" + testControllerMapping
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := os.WriteFile(path, []byte(manual), 0o644); err != nil {
			t.Error(err)
		}
		w.Write([]byte(testControllerMapping))
	}))
	defer server.Close()
	if err := downloadControllerDatabase(context.Background(), path, server.URL, server.Client()); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
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
			if err := downloadControllerDatabase(context.Background(), path, server.URL, server.Client()); err == nil {
				t.Fatal("bad download accepted")
			}
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatalf("bad download published: %v", err)
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
