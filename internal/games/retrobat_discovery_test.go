package games

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestRetroBatMissingInstallClassification(t *testing.T) {
	for _, path := range []string{filepath.Join(t.TempDir(), "RetroBat"), filepath.Join(t.TempDir(), "RetroBat.exe")} {
		if _, err := FindRetroBatExecutable(path); !errors.Is(err, ErrRetroBatNotFound) {
			t.Fatalf("missing installation %q should offer setup, got %v", path, err)
		}
	}
	if _, err := FindRetroBatExecutable(filepath.Join(t.TempDir(), "other.exe")); err == nil || errors.Is(err, ErrRetroBatNotFound) {
		t.Fatalf("wrong executable name must report a config error, got %v", err)
	}
}

func retroBatDiscoveryFixture(t *testing.T) (string, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "RetroBat install")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(dir, "RetroBat.exe")
	if err := os.WriteFile(exe, []byte("discovery fixture; never executed"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir, exe
}

func TestRetroBatConfiguredFolderAndExecutable(t *testing.T) {
	dir, exe := retroBatDiscoveryFixture(t)
	for _, path := range []string{dir, exe, `"` + exe + `"`} {
		got, err := FindRetroBatExecutable(path)
		if err != nil || got != exe {
			t.Fatalf("path %q: got %q, err %v; want %q", path, got, err, exe)
		}
	}
}

func TestRetroBatExplicitPathDoesNotFallBack(t *testing.T) {
	dir, _ := retroBatDiscoveryFixture(t)
	t.Setenv("RETROBAT_PATH", dir)
	for _, path := range []string{filepath.Join(t.TempDir(), "RetroBat.exe"), filepath.Join(dir, "emulationstation.exe")} {
		if exe, err := FindRetroBatExecutable(path); err == nil || exe != "" {
			t.Fatalf("invalid explicit path %q used fallback %q, err %v", path, exe, err)
		}
	}
}

func TestRetroBatEnvironmentSelection(t *testing.T) {
	dir, exe := retroBatDiscoveryFixture(t)
	t.Setenv("RETROBAT_PATH", dir)
	got, err := FindRetroBatExecutable("")
	if err != nil || got != exe {
		t.Fatalf("environment: got %q, err %v; want %q", got, err, exe)
	}
	otherDir, otherExe := retroBatDiscoveryFixture(t)
	got, err = FindRetroBatExecutable(otherDir)
	if err != nil || got != otherExe {
		t.Fatalf("config must win over environment: got %q, err %v; want %q", got, err, otherExe)
	}
	t.Setenv("RETROBAT_PATH", filepath.Join(t.TempDir(), "missing", "RetroBat.exe"))
	if got, err := FindRetroBatExecutable(""); err == nil || got != "" {
		t.Fatalf("invalid environment must fail: got %q, err %v", got, err)
	}
}

func TestRetroBatDiscoveryAfterInstallation(t *testing.T) {
	root := t.TempDir()
	installedDir := filepath.Join(root, "other-drive", "RetroBat")
	candidates := []string{filepath.Join(root, "stale-registry-entry"), installedDir}
	if _, err := findRetroBatExecutable("", "", candidates); !errors.Is(err, ErrRetroBatNotFound) {
		t.Fatalf("before installation: want missing, got %v", err)
	}
	if err := os.MkdirAll(installedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(installedDir, "RetroBat.exe")
	if err := os.WriteFile(exe, []byte("fixture; not executed"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := findRetroBatExecutable("", "", candidates); err != nil || got != exe {
		t.Fatalf("after installation: want %q, got %q, err %v", exe, got, err)
	}
	if _, err := findRetroBatExecutable(filepath.Join(root, "explicit-missing-folder"), "", candidates); !errors.Is(err, ErrRetroBatNotFound) {
		t.Fatalf("explicit selection must stay authoritative, got %v", err)
	}
}

func TestRetroBatRegistryExecutablePath(t *testing.T) {
	for _, scenario := range []struct{ value, want string }{
		{`"D:\RetroBat\retrobat.exe",0`, `D:\RetroBat\retrobat.exe`},
		{`D:\RetroBat folder\retrobat.exe,0`, `D:\RetroBat folder\retrobat.exe`},
		{`"D:\RetroBat\unins000.exe" /SILENT`, `D:\RetroBat\unins000.exe`},
		{`D:\RetroBat\unins000.exe /SILENT`, `D:\RetroBat\unins000.exe`},
		{`"unterminated`, ""},
		{"", ""},
	} {
		if got := retroBatRegistryExecutablePath(scenario.value); got != scenario.want {
			t.Fatalf("parse %q: want %q, got %q", scenario.value, scenario.want, got)
		}
	}
}
