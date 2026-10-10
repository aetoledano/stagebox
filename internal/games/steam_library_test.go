package games

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func writeSteamFixture(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}

func steamManifestFixture(t *testing.T, root string, id, flags int, install string, withFiles bool) string {
	t.Helper()
	path := filepath.Join(root, "steamapps", fmt.Sprintf("appmanifest_%d.acf", id))
	data := fmt.Sprintf(`"AppState" { "appid" "%d" "name" "Game %d" "StateFlags" "%d" "installdir" "%s" "InstalledDepots" { "99" { "manifest" "123" } } }`, id, id, flags, install)
	writeSteamFixture(t, path, []byte(data))
	if withFiles {
		writeSteamFixture(t, filepath.Join(root, "steamapps", "common", install, "game.exe"), []byte("installed"))
	}
	return path
}

func TestSteamVDFEscapesCommentsAndMalformedFiles(t *testing.T) {
	vdf, err := parseSteamVDF("\ufeff" + `// comment
"LibraryFolders" {
 "0" { "path" "D:\\Games\\Steam" "apps" { "42" "999" } }
 "1" "E:\SteamLibrary"
 "empty" ""
 "quoted" "A \"Game\""
}`)
	if err != nil {
		t.Fatal(err)
	}
	folders := vdf.objects["libraryfolders"]
	if folders.objects["0"].values["path"] != `D:\Games\Steam` || folders.values["1"] != `E:\SteamLibrary` || folders.values["quoted"] != `A "Game"` {
		t.Fatalf("escaped Windows paths or quoted names were lost: %+v", folders)
	}
	for _, data := range []string{`"AppState" {`, `"AppState" { "appid" }`, `"bad`, `}`, `"AppState" { "appid" "42"`} {
		if _, err := parseSteamVDF(data); err == nil {
			t.Fatalf("accepted malformed VDF %q", data)
		}
	}
}

func TestSteamOnlyFullyInstalledGamesAcrossLibraries(t *testing.T) {
	root, second := t.TempDir(), t.TempDir()
	steamExe := filepath.Join(root, "Steam.exe")
	writeSteamFixture(t, steamExe, []byte("client"))
	escaped := strings.ReplaceAll(second, `\`, `\\`)
	writeSteamFixture(t, filepath.Join(root, "steamapps", "libraryfolders.vdf"), []byte(fmt.Sprintf(`"LibraryFolders" { "0" { "path" "%s" "apps" { "9999" "100" } } "1" "%s" }`, escaped, escaped)))
	steamManifestFixture(t, root, 42, 4, "Game42", true)
	steamManifestFixture(t, second, 43, 6, "Game43", true) // An update is available, but the game is installed.
	steamManifestFixture(t, second, 42, 4, "Duplicate", true)
	steamManifestFixture(t, root, 44, 1026, "Downloading", true)
	steamManifestFixture(t, root, 45, 4, "RemovedDirectory", false)
	steamManifestFixture(t, root, 46, 5, "Uninstalled", true)
	steamManifestFixture(t, root, 47, 4|1048576, "Partial", true)
	steamManifestFixture(t, root, 228980, 4, "Redistributables", true)
	writeSteamFixture(t, filepath.Join(root, "steamapps", "appmanifest_bad.acf"), []byte(`"AppState" {`))
	var errorMu sync.Mutex
	errors := 0
	progress := []int{}
	loaded := LoadGamesWithOptions(filepath.Join(root, "missing-local-games"), LoadOptions{
		SteamExecutable: steamExe,
		OnError:         func(string, error) { errorMu.Lock(); errors++; errorMu.Unlock() },
		OnProgress: func(done, total int) {
			if total != 2 {
				t.Errorf("total=%d, want 2", total)
			}
			progress = append(progress, done)
		},
	})
	if len(loaded) != 2 {
		t.Fatalf("installed library includes missing/partial/duplicate apps: %+v", loaded)
	}
	for _, game := range loaded {
		if game.SteamAppID != 42 && game.SteamAppID != 43 {
			t.Fatalf("unexpected app: %+v", game)
		}
		if !game.AdvancedTracking || gameTrackingMode(game) != TrackModeSteamGame {
			t.Fatal("Steam game must use its dedicated advanced tracking path")
		}
		if game.ID == deterministicGameID("", game.Name) {
			t.Fatal("Steam IDs must use a separate namespace from local games")
		}
		if err := validateSteamGame(game); err != nil {
			t.Fatal(err)
		}
	}
	if errors != 2 {
		t.Fatalf("expected local-root and malformed-manifest errors, got %d", errors)
	}
	if fmt.Sprint(progress) != "[0 1 2]" {
		t.Fatalf("progress must be monotonic: %v", progress)
	}
}

func TestSteamLaunchRevalidatesInstallation(t *testing.T) {
	root := t.TempDir()
	path := steamManifestFixture(t, root, 42, 4, "Game", true)
	game, err := steamGameFromManifest(path, filepath.Join(root, "Steam.exe"))
	if err != nil || game == nil {
		t.Fatalf("fixture did not load: %v", err)
	}
	if err := os.RemoveAll(game.StartsOn); err != nil {
		t.Fatal(err)
	}
	if err := validateSteamGame(game); err == nil {
		t.Fatal("uninstalled game was allowed to launch")
	}
	steamManifestFixture(t, root, 42, 4, "../Outside", false)
	if _, err := steamGameFromManifest(path, game.Executable); err == nil {
		t.Fatal("accepted path traversal in installdir")
	}
}

func TestSteamArtworkUsesHashedCacheWithoutNetwork(t *testing.T) {
	root := t.TempDir()
	game := &GameData{SteamAppID: 42, ID: deterministicGameID("", "Artwork fixture"), Executable: filepath.Join(root, "Steam.exe")}
	cache := filepath.Join(root, "appcache", "librarycache", "42", "contenthash")
	var jpegData bytes.Buffer
	if err := jpeg.Encode(&jpegData, image.NewRGBA(image.Rect(0, 0, 64, 36)), nil); err != nil {
		t.Fatal(err)
	}
	writeSteamFixture(t, filepath.Join(cache, "library_header.jpg"), jpegData.Bytes())
	writeSteamFixture(t, filepath.Join(cache, "library_600x900.jpg"), jpegData.Bytes())
	prepareSteamArtwork(game, LoadOptions{ThumbnailDir: filepath.Join(root, "thumbs"), ThumbnailURLPrefix: "/game-thumbs"})
	if !strings.HasSuffix(game.Image, "library_header.jpg") || game.ThumbnailURL == "" || game.ImageHash == "" {
		t.Fatalf("local landscape artwork was not used: %+v", game)
	}
}

func TestSteamAppInfoSupportsInlineAndStringTableKeys(t *testing.T) {
	for _, magic := range []uint32{0x07564427, 0x07564428, 0x07564429} {
		t.Run(fmt.Sprintf("%x", magic), func(t *testing.T) {
			var file, blob bytes.Buffer
			keys := []string{"appinfo", "common", "type", "controller_support"}
			key := func(kind byte, index int) {
				blob.WriteByte(kind)
				if magic == 0x07564429 {
					_ = binary.Write(&blob, binary.LittleEndian, uint32(index))
				} else {
					blob.WriteString(keys[index])
					blob.WriteByte(0)
				}
			}
			key(0, 0)
			key(0, 1)
			key(1, 2)
			blob.WriteString("game\x00")
			key(1, 3)
			blob.WriteString("full\x00")
			blob.Write([]byte{8, 8, 8})
			metadataSize := 40
			if magic >= 0x07564428 {
				metadataSize = 60
			}
			_ = binary.Write(&file, binary.LittleEndian, magic)
			_ = binary.Write(&file, binary.LittleEndian, uint32(1))
			if magic == 0x07564429 {
				_ = binary.Write(&file, binary.LittleEndian, uint64(16+8+metadataSize+blob.Len()+4))
			}
			_ = binary.Write(&file, binary.LittleEndian, uint32(42))
			_ = binary.Write(&file, binary.LittleEndian, uint32(metadataSize+blob.Len()))
			file.Write(make([]byte, metadataSize))
			file.Write(blob.Bytes())
			_ = binary.Write(&file, binary.LittleEndian, uint32(0))
			if magic == 0x07564429 {
				_ = binary.Write(&file, binary.LittleEndian, uint32(len(keys)))
				for _, key := range keys {
					file.WriteString(key)
					file.WriteByte(0)
				}
			}
			path := filepath.Join(t.TempDir(), "appinfo.vdf")
			writeSteamFixture(t, path, file.Bytes())
			info, err := readSteamAppInfo(path, map[uint32]bool{42: true})
			if err != nil || info[42].appType != "game" || info[42].controllerSupport != "full" {
				t.Fatalf("metadata=%v err=%v", info, err)
			}
			fileData := file.Bytes()[:file.Len()-1]
			writeSteamFixture(t, path, fileData)
			if _, err := readSteamAppInfo(path, map[uint32]bool{42: true}); err == nil {
				t.Fatal("truncated metadata was accepted")
			}
		})
	}
}

func TestSteamControllerSupportMapsToCouchControls(t *testing.T) {
	for _, test := range []struct {
		support string
		mode    int
	}{
		{"full", 0}, {" Full ", 0}, {"partial", 2}, {"none", 1}, {"", 1},
	} {
		if mode := steamControlMode(test.support); mode != test.mode {
			t.Fatalf("Steam support %q maps to %d, want %d", test.support, mode, test.mode)
		}
	}
}
